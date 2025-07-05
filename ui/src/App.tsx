import { useState } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CssBaseline, ThemeProvider, createTheme } from '@mui/material';
import { Dashboard } from './components/Dashboard';
import { KubernetesDashboard } from './components/KubernetesDashboard';
import { Navigation } from './components/Navigation';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

const theme = createTheme({
  palette: {
    mode: 'dark',
    background: {
      default: '#1e1e2d',
      paper: '#2a2a3e',
    },
    primary: {
      main: '#3f51b5',
    },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundColor: '#1e1e2d',
          color: '#ffffff',
        },
      },
    },
  },
});

function App() {
  const [view, setView] = useState<'system' | 'kubernetes'>('system');

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <Navigation view={view} onViewChange={setView} />
        {view === 'system' ? <Dashboard /> : <KubernetesDashboard />}
      </ThemeProvider>
    </QueryClientProvider>
  );
}

export default App;
