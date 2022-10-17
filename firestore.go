package relay

import (
	"context"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bklimt/relay/common"
	"github.com/bklimt/relay/nest"

	firebase "firebase.google.com/go"
)

func KeyForNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func GenerateStateToken(ctx context.Context, app *firebase.App) (string, error) {
	// Generate a state token.
	client, err := app.Firestore(ctx)
	if err != nil {
		return "", common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer client.Close()

	doc, _, err := client.Collection("auth").Add(ctx, map[string]interface{}{
		"used":    false,
		"created": firestore.ServerTimestamp,
	})
	if err != nil {
		return "", common.Errorf(http.StatusInternalServerError, "unable to write to firestore: %s", err)
	}
	return doc.ID, nil
}

func CheckState(ctx context.Context, app *firebase.App, state string) error {
	fs, err := app.Firestore(ctx)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer fs.Close()

	doc, err := fs.Collection("auth").Doc(state).Get(ctx)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to read state from firestore: %s", err)
	}
	stateData := doc.Data()
	if stateData["used"] != false {
		return common.Errorf(http.StatusForbidden, "invalid oauth state")
	}
	stateData["used"] = true
	stateData["updated"] = firestore.ServerTimestamp
	_, err = fs.Collection("auth").Doc(state).Set(ctx, stateData)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to write state to firestore: %s", err)
	}
	return nil
}

func SaveNestData(ctx context.Context, app *firebase.App, data *nest.Data) error {
	fs, err := app.Firestore(ctx)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer fs.Close()

	userDoc := fs.Collection("user").Doc(data.Metadata.UserID)
	_, err = userDoc.Set(ctx, map[string]interface{}{
		"access_token": data.Metadata.AccessToken,
	})
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to write user data to firestore: %s", err)
	}

	for id, therm := range data.Devices.Thermostats {
		thermDoc := userDoc.Collection("thermostat").Doc(id)
		_, err = thermDoc.Set(ctx, therm)
		if err != nil {
			return common.Errorf(http.StatusInternalServerError, "unable to write thermostat data to firestore: %s", err)
		}
	}

	return nil
}

func LogNestData(ctx context.Context, app *firebase.App, key string, data *nest.Data) error {
	fs, err := app.Firestore(ctx)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer fs.Close()

	for id, therm := range data.Devices.Thermostats {
		name, ok := therm["name"].(string)
		if !ok {
			name = id
		}
		therm["timestamp"] = firestore.ServerTimestamp

		// Log the data for the device itself.
		_, err = fs.Collection("device").Doc(name).Set(ctx, therm)
		if err != nil {
			return common.Errorf(http.StatusInternalServerError, "unable to write to thermostat data to firestore: %s", err)
		}

		// Log the data to the running log.
		_, err = fs.Collection("device").Doc(name).Collection("log").Doc(key).Set(ctx, therm)
		if err != nil {
			return common.Errorf(http.StatusInternalServerError, "unable to write to thermostat log to firestore: %s", err)
		}
	}

	return nil
}

// Returns a map of device name to timestamp.
func GetMostRecentDeviceTimestamps(ctx context.Context, app *firebase.App) (map[string]time.Time, error) {
	fs, err := app.Firestore(ctx)
	if err != nil {
		return nil, common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer fs.Close()

	// Get the list of devices.
	devices, err := fs.Collection("device").Documents(ctx).GetAll()
	if err != nil {
		return nil, common.Errorf(http.StatusInternalServerError, "unable to read devices: %s", err)
	}

	timestamps := map[string]time.Time{}
	for _, device := range devices {
		data := device.Data()
		if timestamp, ok := data["timestamp"].(time.Time); ok {
			timestamps[device.Ref.ID] = timestamp
		} else {
			return nil, common.Errorf(
				http.StatusInternalServerError,
				"device %s has invalid timestamp: %v", device.Ref.ID, data["timestamp"])
		}
	}

	return timestamps, nil
}

func GetNestUsers(ctx context.Context, app *firebase.App) (map[string]string, error) {
	users := map[string]string{}

	fs, err := app.Firestore(ctx)
	if err != nil {
		return users, common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer fs.Close()

	userDocs, err := fs.Collection("user").Documents(ctx).GetAll()
	if err != nil {
		return users, common.Errorf(http.StatusInternalServerError, "unable to query for users: %s", err)
	}

	for _, userDoc := range userDocs {
		id := userDoc.Ref.ID
		accessToken, ok := userDoc.Data()["access_token"]
		if !ok {
			return users, common.Errorf(http.StatusInternalServerError, "user missing access token: %s", id)
		}
		token, ok := accessToken.(string)
		if !ok {
			return users, common.Errorf(http.StatusInternalServerError, "access token was not a string: %v", accessToken)
		}
		users[id] = token
	}

	return users, nil
}

func LogFeatherData(ctx context.Context, app *firebase.App, key string, data map[string]interface{}) error {
	data["timestamp"] = firestore.ServerTimestamp

	client, err := app.Firestore(ctx)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to initialize firestore: %s", err)
	}
	defer client.Close()

	// Save the data for the device itself.
	_, err = client.Collection("device").Doc("feather").Set(ctx, data)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to write data to firestore: %s", err)
	}

	// Save the data to the running log.
	_, err = client.Collection("device").Doc("feather").Collection("log").Doc(key).Set(ctx, data)
	if err != nil {
		return common.Errorf(http.StatusInternalServerError, "unable to write log to firestore: %s", err)
	}

	return nil
}

func InitFirebase(cfg *firebase.Config) *firebase.App {
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("error initializing firebase: %s", err)
	}
	return app
}
