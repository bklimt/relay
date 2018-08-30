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
1. Google service account json, at `GOOGLE_APPLICATION_CREDENTIALS`.
2. relay server config json, as `KLIMT_RELAY_CONFIG`, with this format:

```
{
  "clientId": "Nest client ID",
  "clientSecret": "Nest client secret",
  "projectId": "Firebase project ID"
}
```

## Ports

The server runs in the container on port `:8080`. 