# UI   React Dashboard

VisualEyes frontend. React 19 + MUI + Vite. Live system and Kubernetes monitoring with WebSocket-driven updates.

## Stack

| Library | Version | Purpose |
|---------|---------|---------|
| React | 19 | UI framework |
| MUI | 7 | Component library |
| Vite | 4 | Build tool + dev server |
| TypeScript | 5.8 | Type safety |
| React Query | 5 | Server state, polling |
| Recharts | 3 | Metric charts |
| Axios | 1 | HTTP client |
| React Router | 7 | Client-side routing |

## Dev Setup

```bash
cd ui
npm install
npm run dev
# Available at http://localhost:5173
```

Backend must be running on port 8080. Dev server proxies API calls automatically.

## Build for Production

```bash
npm run build
# Output → ui/dist/
```

Serve with any static file server, or use the Nginx Docker image:

```bash
docker build -f ui/Dockerfile -t visual-eyes-ui:latest ./ui
docker run --rm -p 3000:3000 visual-eyes-ui:latest
```

## Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Dev server with HMR at port 5173 |
| `npm run build` | TypeScript compile + Vite production build |
| `npm run lint` | ESLint with react-hooks and react-refresh plugins |
| `npm run preview` | Preview production build locally |

## Views

| View | Route | Description |
|------|-------|-------------|
| System Dashboard | `/` | CPU, memory, disk, network, load   live charts |
| Kubernetes Dashboard | `/kubernetes` | Pod list, node stats, events |
| Alerts | `/alerts` | Active alerts with severity and timestamps |
| RCA | `/rca/:id` | AI Root Cause Analysis detail drawer |
| Logs | `/logs` | Pod log viewer with live follow |

## Backend Connection

The UI connects to the backend at `http://localhost:8080` by default.

Override via environment variable before building:

```bash
VITE_API_BASE_URL=http://my-backend:8080 npm run build
```

WebSocket live updates connect to `ws://localhost:8080/ws` automatically.

## Theme

Dark and light mode toggle in the top navigation bar. Preference persisted in `localStorage`.
