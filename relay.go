package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"golang.org/x/net/context"

	firebase "firebase.google.com/go"

	"google.golang.org/api/option"
)

// TODO(klimt): Set account default credentials.
const serviceKey = "/home/bklimt/Downloads/home-d09a0-firebase-adminsdk-ofkx1-beb0122e5d.json"

func Serve(port uint16, app *firebase.App) {
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

		firestore, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer firestore.Close()

		_, _, err = firestore.Collection("log").Add(r.Context(), data)
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

func InitFirebase() *firebase.App {
	opt := option.WithCredentialsFile(serviceKey)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("error initializing firebase: %s", err)
	}
	return app
}

func main() {
	app := InitFirebase()
	Serve(8080, app)
}
