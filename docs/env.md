# Environment Variable Overrides

The server uses config/config.yaml for most settings. The following environment variables override runtime behavior:

- `DEBUG`: Set to `true` to enable development logging output.
- `VITE_DEV_SERVER`: When `dev_mode` is true, overrides the Vite dev server URL used for proxying (default: `http://127.0.0.1:5173`).
