# Relay Server

A go server for routing some home automation data to Firestore.

## Building an Image

Build the docker image:
```
sudo docker build --no-cache -t relay ./
```

Export it to a tar file:
```
sudo docker save -o relay.tar relay
```

## Config Files

There are two environment variables that must point to config files needed to run the server.
1. Google service account json, from Firebase console.
    * Environment variable: `GOOGLE_APPLICATION_CREDENTIALS`
    * Default location: `/etc/relay/credentials.json`
2. relay server config json
    * Environment variable: `KLIMT_RELAY_CONFIG`
    * Default location: `/etc/relay/config.json`
    * Format:
```
{
  "clientId": "Nest client ID",
  "clientSecret": "Nest client secret",
  "projectId": "Firebase project ID"
}
```

## Ports

The server runs in the container on port `:8080`. 