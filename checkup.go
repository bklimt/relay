package relay

import (
	"context"
	"expvar"
	"log"
	"time"

	firebase "firebase.google.com/go"
)

var (
	checkupIntervalSeconds *expvar.Int    = expvar.NewInt("checkupIntervalSeconds")
	lastCheckupTime        *expvar.String = expvar.NewString("lastCheckupTime")
)

func Checkup(ctx context.Context, app *firebase.App) {
	timestamp := KeyForNow()
	log.Printf("%s: Checkup.", timestamp)
	lastCheckupTime.Set(timestamp)

	// Grab the most recent logs.
	timestamps, err := GetMostRecentDeviceTimestamps(ctx, app)
	if err != nil {
		log.Printf("Unable to get most recent log data: %s\n", err)
	}
	log.Printf("Device Timestamps: %v", timestamps)

	now := time.Now().UTC()
	for device, timestamp := range timestamps {
		timeSince := now.Sub(timestamp)
		if timeSince.Hours() > 1 {
			log.Printf("Device %s has not responded for >1 hour.", device)
		}
	}
}

func CheckupForever(app *firebase.App, cfg *Config) {
	ctx := context.Background()
	checkupIntervalSeconds.Set(int64(cfg.CheckupIntervalSeconds))
	for {
		Checkup(ctx, app)
		interval := time.Duration(checkupIntervalSeconds.Value()) * time.Second
		time.Sleep(interval)
	}
}
