package nest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/bklimt/relay/common"
)

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type Thermostat map[string]interface{}

/*
type Thermostat struct {
	Timestamp                 interface{} // The time the doc was saved.
	Humidity                  int         `json:"humidity" firestore:"humidity"`
	Locale                    string      `json:"locale" firestore:"locale"`
	TemperatureScale          string      `json:"temperature_scale" firestore:"temperature_scale"`
	IsUsingEmergencyHeat      bool        `json:"is_using_emergency_heat" firestore:"is_using_emergency_heat"`
	HasFan                    bool        `json:"has_fan" firestore:"has_fan"`
	SoftwareVersion           string      `json:"software_version" firestore:"software_version"`
	HasLeaf                   bool        `json:"has_leaf" firestore:"has_leaf"`
	DeviceID                  string      `json:"device_id" firestore:"device_id"`
	Name                      string      `json:"name" firestore:"name"`
	CanHeat                   bool        `json:"can_heat" firestore:"can_heat"`
	CanCool                   bool        `json:"can_cool" firestore:"can_cool"`
	TargetTemperatureC        float32     `json:"target_temperature_c" firestore:"target_temperature_c"`
	TargetTemperatureF        float32     `json:"target_temperature_f" firestore:"target_temperature_f"`
	TargetTemperatureHighC    float32     `json:"target_temperature_high_c" firestore:"target_temperature_high_c"`
	TargetTemperatureHighF    float32     `json:"target_temperature_high_f" firestore:"target_temperature_high_f"`
	TargetTemperatureLowC     float32     `json:"target_temperature_low_c" firestore:"target_temperature_low_c"`
	TargetTemperatureLowF     float32     `json:"target_temperature_low_f" firestore:"target_temperature_low_f"`
	AmbientTemperatureC       float32     `json:"ambient_temperature_c" firestore:"ambient_temperature_c"`
	AmbientTemperatureF       float32     `json:"ambient_temperature_f" firestore:"ambient_temperature_f"`
	AwayTemperatureHighC      float32     `json:"away_temperature_high_c" firestore:"away_temperature_high_c"`
	AwayTemperatureHighF      float32     `json:"away_temperature_high_f" firestore:"away_temperature_high_f"`
	AwayTemperatureLowC       float32     `json:"away_temperature_low_c" firestore:"away_temperature_low_c"`
	AwayTemperatureLowF       float32     `json:"away_temperature_low_f" firestore:"away_temperature_low_f"`
	EcoTemperatureHighC       float32     `json:"eco_temperature_high_c" firestore:"eco_temperature_high_c"`
	EcoTemperatureHighF       float32     `json:"eco_temperature_high_f" firestore:"eco_temperature_high_f"`
	EcoTemperatureLowC        float32     `json:"eco_temperature_low_c" firestore:"eco_temperature_low_c"`
	EcoTemperatureLowF        float32     `json:"eco_temperature_low_f" firestore:"eco_temperature_low_f"`
	IsLocked                  bool        `json:"is_locked" firestore:"is_locked"`
	LockedTempMinC            float32     `json:"locked_temp_min_c" firestore:"locked_temp_min_c"`
	LockedTempMinF            float32     `json:"locked_temp_min_f" firestore:"locked_temp_min_f"`
	LockedTempMaxC            float32     `json:"locked_temp_max_c" firestore:"locked_temp_max_c"`
	LockedTempMaxF            float32     `json:"locked_temp_max_f" firestore:"locked_temp_max_f"`
	SunlightCorrectionActive  bool        `json:"sunlight_correction_active" firestore:"sunlight_correction_active"`
	SunlightCorrectionEnabled bool        `json:"sunlight_correction_enabled" firestore:"sunlight_correction_enabled"`
	StructureID               string      `json:"structure_id" firestore:"structure_id"`
	FanTimerActive            bool        `json:"fan_timer_active" firestore:"fan_timer_active"`
	FanTimerTimeout           string      `json:"fan_timer_timeout" firestore:"fan_timer_timeout"`
	FanTimerDuration          int         `json:"fan_timer_duration" firestore:"fan_timer_duration"`
	PreviousHVACMode          string      `json:"previous_hvac_mode" firestore:"previous_hvac_mode"`
	HVACMode                  string      `json:"hvac_mode" firestore:"hvac_mode"`
	TimeToTarget              string      `json:"time_to_target" firestore:"time_to_target"`
	TimeToTargetTraining      string      `json:"time_to_target_training" firestore:"time_to_target_training"`
	Label                     string      `json:"label" firestore:"label"`
	NameLong                  string      `json:"name_long" firestore:"name_long"`
	IsOnline                  bool        `json:"is_online" firestore:"is_online"`
	LastConnection            string      `json:"last_connection" firestore:"last_connection"`
	HVACState                 string      `json:"hvac_state" firestore:"hvac_state"`
}
*/

type Data struct {
	Devices struct {
		Thermostats map[string]Thermostat `json:"thermostats"`
	} `json:"devices"`
	Metadata struct {
		UserID        string `json:"user_id"`
		AccessToken   string `json:"access_token"`
		ClientVersion int    `json:"client_version"`
	} `json:"metadata"`
	Structures map[string]interface{} `json:"structures"`
}

func GetAccessToken(clientID string, clientSecret string, code string) (string, error) {
	// Get a Nest access token.
	const tokenURL = "https://api.home.nest.com/oauth2/access_token"
	values := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}
	// TODO(klimt): This should probably take a context.
	response, err := http.PostForm(tokenURL, values)
	if err != nil {
		return "", common.Errorf(http.StatusInternalServerError, "unable to connect to nest: %s", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", common.Errorf(http.StatusForbidden, "unable to get access token: %s", response.Status)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", common.Errorf(http.StatusInternalServerError, "unable to read access token body: %s", err)
	}
	atr := &accessTokenResponse{}
	if err := json.Unmarshal(body, &atr); err != nil {
		return "", common.Errorf(http.StatusInternalServerError, "unable to parse json: %s", err)
	}
	return atr.AccessToken, nil
}

func GetData(ctx context.Context, accessToken string) (*Data, error) {
	const url = "https://developer-api.nest.com"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, common.Errorf(http.StatusInternalServerError, "unable to create request: %s", err)
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
		return nil, common.Errorf(http.StatusInternalServerError, "unable to connect to nest: %s", err)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, common.Errorf(http.StatusInternalServerError, "unable to read metadata body: %s", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, common.Errorf(http.StatusForbidden, "unable to get metadata: %s: %s", response.Status, body)
	}
	var data *Data
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, common.Errorf(http.StatusInternalServerError, "unable to parse json: %s", err)
	}
	return data, nil
}
