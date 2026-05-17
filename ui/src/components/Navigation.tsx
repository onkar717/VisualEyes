import React from 'react';
import {
  AppBar,
  Toolbar,
  Typography,
  Box,
  ToggleButtonGroup,
  ToggleButton,
  IconButton,
  Tooltip,
  Badge,
} from '@mui/material';
import {
  Computer,
  Storage,
  LightMode,
  DarkMode,
  NotificationsActive,
  Article,
} from '@mui/icons-material';
import { useQuery } from '@tanstack/react-query';
import { useTheme } from '../theme/ThemeContext';
import { api } from '../services/api';

export type AppView = 'system' | 'kubernetes' | 'alerts' | 'logs';

interface NavigationProps {
  view: AppView;
  onViewChange: (view: AppView) => void;
}

export const Navigation: React.FC<NavigationProps> = ({ view, onViewChange }) => {
  const { isDarkMode, toggleTheme } = useTheme();

  // Live firing-alert count for the badge on the Alerts tab.
  const { data: firingAlerts = [] } = useQuery({
    queryKey: ['alerts', 'firing'],
    queryFn: () => api.getAlerts('firing'),
    refetchInterval: 15_000,
  });
  const firingCount = firingAlerts.length;

  const handleChange = (
    _event: React.MouseEvent<HTMLElement>,
    newView: AppView | null,
  ) => {
    if (newView !== null) onViewChange(newView);
  };

  const btnSx = {
    color: isDarkMode ? '#fff' : '#2c3e50',
    padding: '6px 16px',
    borderRadius: '8px',
    transition: 'all 0.3s ease',
    border: 'none',
    margin: '0 2px',
    fontSize: '0.82rem',
    '&.Mui-selected': {
      backgroundColor: '#3f51b5',
      color: '#fff',
      boxShadow: '0 4px 12px rgba(63,81,181,0.3)',
      '&:hover': { backgroundColor: '#3949ab' },
    },
    '&:hover': {
      backgroundColor: isDarkMode ? 'rgba(63,81,181,0.1)' : 'rgba(63,81,181,0.05)',
      transform: 'translateY(-1px)',
    },
  };

  return (
    <AppBar
      position="fixed"
      sx={{
        boxShadow: '0 4px 6px rgba(0,0,0,0.1)',
        backdropFilter: 'blur(10px)',
        background: isDarkMode
          ? 'linear-gradient(180deg, rgba(26,26,46,0.95) 0%, rgba(26,26,46,0.9) 100%)'
          : 'linear-gradient(180deg, rgba(255,255,255,0.95) 0%, rgba(255,255,255,0.9) 100%)',
      }}
    >
      <Toolbar>
        <Typography
          variant="h6"
          component="div"
          sx={{ flexGrow: 1, fontWeight: 600, letterSpacing: '0.5px', color: isDarkMode ? '#fff' : '#2c3e50' }}
        >
          VisualEyes
        </Typography>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <ToggleButtonGroup
            value={view}
            exclusive
            onChange={handleChange}
            aria-label="view selector"
            sx={{
              backgroundColor: isDarkMode ? 'rgba(42,42,62,0.5)' : 'rgba(245,245,245,0.5)',
              padding: '4px',
              borderRadius: '12px',
              border: isDarkMode ? '1px solid rgba(255,255,255,0.1)' : '1px solid rgba(0,0,0,0.1)',
              backdropFilter: 'blur(8px)',
              '& .MuiToggleButton-root': btnSx,
            }}
          >
            <ToggleButton value="system" aria-label="system metrics">
              <Computer sx={{ mr: 0.75, fontSize: 18 }} />
              System
            </ToggleButton>
            <ToggleButton value="kubernetes" aria-label="kubernetes metrics">
              <Storage sx={{ mr: 0.75, fontSize: 18 }} />
              Kubernetes
            </ToggleButton>
            <ToggleButton value="alerts" aria-label="alerts">
              <Badge badgeContent={firingCount || null} color="error" sx={{ mr: 0.75 }}>
                <NotificationsActive sx={{ fontSize: 18 }} />
              </Badge>
              Alerts
            </ToggleButton>
            <ToggleButton value="logs" aria-label="pod logs">
              <Article sx={{ mr: 0.75, fontSize: 18 }} />
              Logs
            </ToggleButton>
          </ToggleButtonGroup>

          <Tooltip title={`Switch to ${isDarkMode ? 'Light' : 'Dark'} Mode`}>
            <IconButton
              onClick={toggleTheme}
              sx={{
                color: isDarkMode ? '#fff' : '#2c3e50',
                backgroundColor: isDarkMode ? 'rgba(42,42,62,0.5)' : 'rgba(245,245,245,0.5)',
                borderRadius: '12px',
                padding: '8px',
                transition: 'all 0.3s ease',
                '&:hover': {
                  backgroundColor: isDarkMode ? 'rgba(63,81,181,0.1)' : 'rgba(63,81,181,0.05)',
                  transform: 'translateY(-1px)',
                },
              }}
            >
              {isDarkMode ? <LightMode /> : <DarkMode />}
            </IconButton>
          </Tooltip>
        </Box>
      </Toolbar>
    </AppBar>
  );
};