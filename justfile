# Dev tasks (fork only, see https://just.systems). Run `just` to list them.

air := "github.com/air-verse/air@v1.67.4"
keycloak_kube := "dev/keycloak.kube.yaml"

# List recipes
default:
    @just --list

# Run backend and frontend together, both with hot reload (Ctrl-C stops both)
dev:
    #!/usr/bin/env bash
    set -uo pipefail
    trap 'trap - EXIT; kill $(jobs -p) 2>/dev/null; wait' EXIT
    just dev-server &
    just dev-ui &
    wait -n

# Run the backend in DEV mode, rebuilt and restarted on Go changes (air)
[working-directory('server')]
dev-server:
    MODE=DEV exec go run {{ air }}

# Run the frontend Vite dev server (pass `--host` to expose it on the LAN)
dev-ui *args:
    ./node_modules/.bin/vite dev {{ args }}

# Start (or reset) the local Keycloak to test OIDC auth, see dev/keycloak.kube.yaml
keycloak:
    podman kube play --replace {{ keycloak_kube }}
    @echo "Keycloak: http://localhost:8180/admin (admin / admin), realm 'watcharr'"

# Stop and remove the local Keycloak
keycloak-down:
    podman kube play --down {{ keycloak_kube }}

# Follow the local Keycloak logs
keycloak-logs:
    podman logs -f watcharr-keycloak-keycloak
