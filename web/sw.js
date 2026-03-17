const CACHE='turbopos-v3';
self.addEventListener('install',()=>self.skipWaiting());
self.addEventListener('activate',e=>{
  e.waitUntil(
    caches.keys().then(ks=>Promise.all(ks.map(k=>caches.delete(k))))
    .then(()=>self.clients.claim())
  );
});
self.addEventListener('fetch',e=>{
  if(e.request.url.includes('/api/')) return;
  if(e.request.method!=='GET') return;
});
