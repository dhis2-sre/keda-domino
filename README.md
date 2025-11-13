# KEDA Domino

The application will listen for KEDA events and trigger a domino effect by either scaling up or down companion pods such as the database or file storage.

## Motivation

I couldn't get this to work with the KEDA HTTP scaler or using the https://keda.sh/docs/2.18/scalers/kubernetes-workload/ since it requires the ...

## Deploy

### Kubernetes

```shell
skaffold run
```

### Docker

```shell
docker compose up
```

## Development

```shell
make dev
```
