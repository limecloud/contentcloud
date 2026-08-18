import { contextBridge, ipcRenderer } from 'electron';

import { createDesktopApi } from './api';

const api = createDesktopApi(ipcRenderer);
contextBridge.exposeInMainWorld('contentcloudDesktop', api);
