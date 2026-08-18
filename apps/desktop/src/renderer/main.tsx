import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { DesktopApp } from './App';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <DesktopApp />
  </StrictMode>,
);
