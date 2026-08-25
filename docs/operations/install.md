# Install Supabase Manager

Supabase Manager runs as two containers but exposes one browser URL. `manager` serves the React application and API. The private `provisioner` owns the Docker socket and has no published port.

## Prerequisites

- Docker Engine 24+ with Docker Compose v2
- At least 8 GB RAM assigned to Docker (12 GB recommended for Supabase plus builds)
- An absolute project directory that Docker can share with containers

## Configure and start

From the repository root:

```sh
cp deploy/.env.example deploy/.env
mkdir -p /Users/Shared/supabase-manager/projects
openssl rand -base64 32
openssl rand -hex 32
```

Paste the generated values into `deploy/.env`. Set `PROJECT_ROOT` to the absolute directory you created. For a remote HTTPS URL, set `PUBLIC_ORIGIN` to that exact origin and `SECURE_COOKIES=true`.

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build --wait
```

Open `PUBLIC_ORIGIN` (by default `http://localhost:8080`). The first visit creates the administrator and displays one-time recovery codes.

## Operations

```sh
docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps
docker compose -f deploy/docker-compose.yml --env-file deploy/.env logs --tail=200 manager provisioner
docker compose -f deploy/docker-compose.yml --env-file deploy/.env down
```

The last command stops the control plane without deleting Manager data or any Supabase project data. Do not add `-v` unless deleting the Manager database is intentional.
