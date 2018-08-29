package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"golang.org/x/net/context"

	firebase "firebase.google.com/go"
)

type config struct {
	ClientID     string `json:"clientId"`     // The Nest client ID.
	ClientSecret string `json:"clientSecret"` // The Nest client secret.
	ProjectID    string `json:"projectId"`    // The Firebase project ID.
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type thermostat struct {
	Humidity                  int     `json:"humidity"`
	Locale                    string  `json:"locale"`
	TemperatureScale          string  `json:"temperature_scale"`
	IsUsingEmergencyHeat      bool    `json:"is_using_emergency_heat"`
	HasFan                    bool    `json:"has_fan"`
	SoftwareVersion           string  `json:"software_version"`
	HasLeaf                   bool    `json:"has_leaf"`
	DeviceID                  string  `json:"device_id"`
	Name                      string  `json:"name"`
	CanHeat                   bool    `json:"can_heat"`
	CanCool                   bool    `json:"can_cool"`
	TargetTemperatureC        float32 `json:"target_temperature_c"`
	TargetTemperatureF        float32 `json:"target_temperature_f"`
	TargetTemperatureHighC    float32 `json:"target_temperature_high_c"`
	TargetTemperatureHighF    float32 `json:"target_temperature_high_f"`
	TargetTemperatureLowC     float32 `json:"target_temperature_low_c"`
	TargetTemperatureLowF     float32 `json:"target_temperature_low_f"`
	AmbientTemperatureC       float32 `json:"ambient_temperature_c"`
	AmbientTemperatureF       float32 `json:"ambient_temperature_f"`
	AwayTemperatureHighC      float32 `json:"away_temperature_high_c"`
	AwayTemperatureHighF      float32 `json:"away_temperature_high_f"`
	AwayTemperatureLowC       float32 `json:"away_temperature_low_c"`
	AwayTemperatureLowF       float32 `json:"away_temperature_low_f"`
	EcoTemperatureHighC       float32 `json:"eco_temperature_high_c"`
	EcoTemperatureHighF       float32 `json:"eco_temperature_high_f"`
	EcoTemperatureLowC        float32 `json:"eco_temperature_low_c"`
	EcoTemperatureLowF        float32 `json:"eco_temperature_low_f"`
	IsLocked                  bool    `json:"is_locked"`
	LockedTempMinC            float32 `json:"locked_temp_min_c"`
	LockedTempMinF            float32 `json:"locked_temp_min_f"`
	LockedTempMaxC            float32 `json:"locked_temp_max_c"`
	LockedTempMaxF            float32 `json:"locked_temp_max_f"`
	SunlightCorrectionActive  bool    `json:"sunlight_correction_active"`
	SunlightCorrectionEnabled bool    `json:"sunlight_correction_enabled"`
	StructureID               string  `json:"structure_id"`
	FanTimerActive            bool    `json:"fan_timer_active"`
	FanTimerTimeout           string  `json:"fan_timer_timeout"`
	FanTimerDuration          int     `json:"fan_timer_duration"`
	PreviousHVACMode          string  `json:"previous_hvac_mode"`
	HVACMode                  string  `json:"hvac_mode"`
	TimeToTarget              string  `json:"time_to_target"`
	TimeToTargetTraining      string  `json:"time_to_target_training"`
	Label                     string  `json:"label"`
	NameLong                  string  `json:"name_long"`
	IsOnline                  bool    `json:"is_online"`
	LastConnection            string  `json:"last_connection"`
	HVACState                 string  `json:"hvac_state"`
}

type nestData struct {
	Devices struct {
		Thermostats map[string]thermostat `json:"thermostats"`
	} `json:"devices"`
	Metadata struct {
		UserID        string `json:"user_id"`
		AccessToken   string `json:"access_token"`
		ClientVersion int    `json:"client_version"`
	} `json:"metadata"`
	Structures map[string]interface{} `json:"structures"`
}

func serve(port uint16, app *firebase.App, cfg *config) {
	// Redirects to the Nest login.
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

		// Generate a state token.
		client, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer client.Close()

		doc, _, err := client.Collection("auth").Add(r.Context(), map[string]interface{}{
			"used":    false,
			"created": firestore.ServerTimestamp,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}
		state := doc.ID

		// Redirect to the Nest oauth endpoint.
		authURL := fmt.Sprintf("https://home.nest.com/login/oauth2?client_id=%s&state=%s", cfg.ClientID, state)
		w.Header().Add("Location", authURL)
		w.WriteHeader(http.StatusFound)
		fmt.Fprintln(w, "Redirecting")
	})

	// Handles the oauth redirect from Nest.
	http.HandleFunc("/oauth", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to /oauth.\n", r.Method)

		params := r.URL.Query()
		codes := params["code"]
		states := params["state"]
		if len(codes) == 0 || len(states) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "missing code or state")
			return
		}
		code := codes[0]
		state := states[0]

		// Check the state.
		fs, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer fs.Close()

		doc, err := fs.Collection("auth").Doc(state).Get(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to read from firestore: %s", err)
			return
		}
		stateData := doc.Data()
		if stateData["used"] != false {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "invalid oauth state")
			return
		}
		stateData["used"] = true
		stateData["updated"] = firestore.ServerTimestamp
		_, err = fs.Collection("auth").Doc(state).Set(r.Context(), stateData)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}

		// Get a Nest access token.
		accessToken, status, err := getAccessToken(code, cfg)
		if err != nil {
			w.WriteHeader(status)
			fmt.Fprintf(w, "%s", err)
			return
		}

		// Get the user ID.
		data, status, err := getMetadata(r.Context(), accessToken)
		if err != nil {
			w.WriteHeader(status)
			fmt.Fprintf(w, "%s", err)
			return
		}

		// Save the metadata to Firestore.
		userDoc := fs.Collection("user").Doc(data.Metadata.UserID)
		_, err = userDoc.Set(r.Context(), map[string]interface{}{
			"access_token": data.Metadata.AccessToken,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write user data to firestore: %s", err)
			return
		}

		for id, therm := range data.Devices.Thermostats {
			thermDoc := userDoc.Collection("thermostat").Doc(id)
			_, err = thermDoc.Set(r.Context(), therm)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "unable to write thermostat data to firestore: %s", err)
				return
			}
		}

		fmt.Fprintln(w, "Logged in successfully.")
	})

	// Logs a data snapshot to Firestore.
	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unable to read body: %s", err)
			return
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unable to parse json: %s", err)
			return
		}

		key := time.Now().UTC().Format(time.RFC3339)
		data["timestamp"] = firestore.ServerTimestamp

		client, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer client.Close()

		_, err = client.Collection("log").Doc(key).Set(r.Context(), data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}

		fmt.Fprintln(w, "ack")
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Listening on %s.\n", addr)

	log.Fatal(http.ListenAndServe(addr, nil))
}

func getAccessToken(code string, cfg *config) (string, int, error) {
	// Get a Nest access token.
	const tokenURL = "https://api.home.nest.com/oauth2/access_token"
	values := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}
	// TODO(klimt): This should probably take a context.
	response, err := http.PostForm(tokenURL, values)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("unable to connect to nest: %s", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", http.StatusForbidden, fmt.Errorf("unable to get access token: %s", response.Status)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("unable to read access token body: %s", err)
	}
	atr := &accessTokenResponse{}
	if err := json.Unmarshal(body, &atr); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("unable to parse json: %s", err)
	}
	return atr.AccessToken, http.StatusOK, nil
}

func getMetadata(ctx context.Context, accessToken string) (*nestData, int, error) {
	const url = "https://developer-api.nest.com"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("unable to create request: %s", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{
		CheckRedirect: func(redirRequest *http.Request, via []*http.Request) error {
			// Go's http.DefaultClient does not forward headers when a redirect 3xx
			// response is received. Thus, the header (which in this case contains the
			// Authorization token) needs to be passed forward to the redirect
			// destinations.
			redirRequest.Header = req.Header

			// Go's http.DefaultClient allows 10 redirects before returning an
			// an error. We have mimicked this default behavior.
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("unable to connect to nest: %s", err)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("unable to read metadata body: %s", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, http.StatusForbidden, fmt.Errorf("unable to get metadata: %s: %s", response.Status, body)
	}
	var data *nestData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("unable to parse json: %s", err)
	}
	return data, http.StatusOK, nil
}

func loadConfig() *config {
	path := os.Getenv("KLIMT_RELAY_CONFIG")
	if path == "" {
		log.Fatal("KLIMT_RELAY_CONFIG environment variable must point to config")
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("error opening config at %q: %s", path, err)
	}

	var cfg *config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalf("error parsing config %q: %s", data, err)
	}

	return cfg
}

func initFirebase(projectID string) *firebase.App {
	ctx := context.Background()
	cfg := &firebase.Config{ProjectID: projectID}
	app, err := firebase.NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("error initializing firebase: %s", err)
	}
	return app
}

func main() {
	cfg := loadConfig()
	app := initFirebase(cfg.ProjectID)
	serve(8080, app, cfg)
}
