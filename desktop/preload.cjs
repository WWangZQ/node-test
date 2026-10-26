const {contextBridge, ipcRenderer} = require('electron');
contextBridge.exposeInMainWorld('nodeTest', {
  send: data => ipcRenderer.send('node-test:command', data),
  onEvent: listener => ipcRenderer.on('node-test:event', (_event, data) => listener(data))
});
