# ADR 0002: Publish the development broker only on loopback

Date: 2026-09-05

Status: accepted

## Context

The bundled Compose broker has no authentication or transport encryption. It
is intended for local examples and tests, but publishing `9092:9092` binds the
port on every host interface. Other machines could reach the broker without
the local user deliberately enabling remote access.

## Decision

Publish `127.0.0.1:9092:9092`. Keep Kafka's container listener and advertised
`localhost:9092` address unchanged. The local CLI and smoke-test commands keep
their existing defaults.

## Consequences

The host port accepts local connections only. Local users and containers on
the Compose network remain inside the trust boundary; loopback is not user
authentication. Remote access requires a separately secured deployment. This
project does not configure broker TLS or ACLs, and the development broker must
not be exposed to an untrusted network. Existing deployments must recreate
the Compose service to apply the binding change; the archive volume survives.
