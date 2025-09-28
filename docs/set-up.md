markdown
## Connecting Prometheus to Grafana

1. Open Grafana and go to **Settings > Data Sources**.
2. Click **Add data source** and select **Prometheus**.
3. Set the URL to your Prometheus server (e.g., `http://localhost:9090`). or http://prometheus:9090 if use docker-compose.
4. Click **Save & Test** to verify the connection.

## Importing a Dashboard from File

1. In Grafana, go to **Dashboards > Import**.
2. Click **Upload JSON file** and select the dashboard file from this folder.
3. Click **Import** to add the dashboard.w