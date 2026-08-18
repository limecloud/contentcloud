'use strict';

const clientCapabilities = new Map();

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));
self.addEventListener('message', (event) => {
  if (event.data?.type !== 'workbench-capability' || typeof event.data.capability !== 'string' || !event.source?.id) return;
  clientCapabilities.set(event.source.id, event.data.capability);
  event.ports[0]?.postMessage({type: 'workbench-capability-ready'});
});
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith('/api/v1/resources/')) return;
  event.respondWith((async () => {
    const capability = await requestCapability(event.clientId);
    if (!capability) return new Response(JSON.stringify({error: {code: 'WORKBENCH_CAPABILITY_INVALID', message: '资源 capability 尚未就绪'}}), {status: 401, headers: {'Content-Type': 'application/json'}});
    const headers = new Headers(event.request.headers);
    headers.set('Authorization', `Bearer ${capability}`);
    return fetch(new Request(event.request, {headers, cache: 'no-store'}));
  })());
});

async function requestCapability(clientID) {
  let client = clientID ? await self.clients.get(clientID) : null;
  if (!client) {
    const windows = await self.clients.matchAll({type: 'window', includeUncontrolled: true});
    if (windows.length !== 1) return '';
    [client] = windows;
  }
  const cached = clientCapabilities.get(client.id);
  if (cached) return cached;
  return new Promise((resolve) => {
    const channel = new MessageChannel();
    const finish = (value) => { clearTimeout(timeout); channel.port1.close(); resolve(value); };
    const timeout = setTimeout(() => finish(''), 3000);
    channel.port1.addEventListener('message', (event) => {
      const value = event.data?.type === 'workbench-capability-response' ? event.data.capability : '';
      const capability = typeof value === 'string' ? value : '';
      if (capability) clientCapabilities.set(client.id, capability);
      finish(capability);
    }, {once: true});
    channel.port1.start();
    client.postMessage({type: 'workbench-capability-request'}, [channel.port2]);
  });
}
