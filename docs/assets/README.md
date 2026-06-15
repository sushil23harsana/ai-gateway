# README assets

Drop the dashboard screenshot here as **`dashboard.png`** (a GIF works too), then
add this line to the main [README](../../README.md#dashboard):

```markdown
![Janus dashboard](docs/assets/dashboard.png)
```

## How to capture (≈2 minutes)

```bash
# 1. real secrets in .env (ADMIN_TOKEN via `make gen-token`, OPENAI_API_KEY)
docker compose up -d --build

# 2. mint a key and push a little traffic so the charts have data
KEY=$(curl -s -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"demo","rate_limit_rpm":60,"monthly_budget_usd":5}' | jq -r .key)
for i in $(seq 1 8); do
  curl -s http://localhost:8080/v1/chat/completions \
    -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello '"$i"'"}]}' >/dev/null
done
```

3. Open <http://localhost:3000>, screenshot the overview (and/or the Keys page),
   and save it here as `dashboard.png`.

This is the one BUILD.md §8 item that needs a real running instance — it can't be
faked, so it's left for you to capture from your own data.
