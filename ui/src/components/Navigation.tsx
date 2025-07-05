import React from 'react';
import { AppBar, Toolbar, Typography, Box, ToggleButton, ToggleButtonGroup } from '@mui/material';
import { Computer, Storage } from '@mui/icons-material';

interface NavigationProps {
  view: 'system' | 'kubernetes';
  onViewChange: (view: 'system' | 'kubernetes') => void;
}

export const Navigation: React.FC<NavigationProps> = ({ view, onViewChange }) => {
  const handleChange = (
    _event: React.MouseEvent<HTMLElement>,
    newView: 'system' | 'kubernetes' | null
  ) => {
    if (newView !== null) {
      onViewChange(newView);
    }
  };

  return (
    <AppBar position="static" sx={{ backgroundColor: '#1a1a2e', boxShadow: 'none', borderBottom: '1px solid #2a2a3e' }}>
      <Toolbar>
        <Typography variant="h6" component="div" sx={{ flexGrow: 1, color: '#fff' }}>
          VisualEyes Dashboard
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <ToggleButtonGroup
            value={view}
            exclusive
            onChange={handleChange}
            aria-label="view selector"
            sx={{
              backgroundColor: '#2a2a3e',
              '& .MuiToggleButton-root': {
                color: '#fff',
                '&.Mui-selected': {
                  backgroundColor: '#3f51b5',
                  color: '#fff',
                  '&:hover': {
                    backgroundColor: '#3f51b5',
                  },
                },
                '&:hover': {
                  backgroundColor: '#3a3a4e',
                },
              },
            }}
          >
            <ToggleButton value="system" aria-label="system metrics">
              <Computer sx={{ mr: 1 }} />
              System
            </ToggleButton>
            <ToggleButton value="kubernetes" aria-label="kubernetes metrics">
              <Storage sx={{ mr: 1 }} />
              Kubernetes
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>
      </Toolbar>
    </AppBar>
  );
}; 