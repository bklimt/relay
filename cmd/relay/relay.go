package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/bklimt/relay"
	"github.com/bklimt/relay/common"
	"github.com/bklimt/relay/nest"

	firebase "firebase.google.com/go"
)

func serve(port uint16, app *firebase.App, cfg *relay.Config) {
	// Redirects to the Nest login.
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

		// Generate a state token.
		state, err := relay.GenerateStateToken(r.Context(), app)
		if err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

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
		if err := relay.CheckState(r.Context(), app, state); err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

		// Get a Nest access token.
		accessToken, err := nest.GetAccessToken(cfg.ClientID, cfg.ClientSecret, code)
		if err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

		// Get the user ID.
		data, err := nest.GetData(r.Context(), accessToken)
		if err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

		// Save the metadata to Firestore.
		if err := relay.SaveNestData(r.Context(), app, data); err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
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

		key := relay.KeyForNow()

		if err := relay.LogFeatherData(r.Context(), app, key, data); err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

		if err := LogNestData(r.Context(), app, key); err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
			return
		}

		fmt.Fprintln(w, "ack")
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Listening on %s.\n", addr)

	log.Fatal(http.ListenAndServe(addr, nil))
}

func LogNestData(ctx context.Context, app *firebase.App, key string) error {
	users, err := relay.GetNestUsers(ctx, app)
	if err != nil {
		return err
	}

	for _, token := range users {
		data, err := nest.GetData(ctx, token)
		if err != nil {
			return err
		}

		err = relay.LogNestData(ctx, app, key, data)
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	cfg := relay.LoadConfig()
	app := relay.InitFirebase(cfg.ProjectID)
	serve(8080, app, cfg)
}
