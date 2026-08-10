# Amber Desk

Browser-based OSINT workspace prototype with an amber CRT interface and a small Go backend.

## Run

```powershell
go run .
```

Open <http://localhost:8080>. Set a different address when the port is occupied:

```powershell
$env:ADDR = ":8081"
go run .
```

## API

- `GET /api/health`
- `GET /api/case`
- `PATCH /api/events/{id}/status`
- `POST /api/events/{id}/notes`

The current store is in memory and resets when the server restarts.
