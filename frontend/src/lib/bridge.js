const demo = {
  revision: 1,
  items: [
    { id:'1', name:'A001_C001_0722AB.mov', size:18790481920, uploaded:13287555072, status:'uploading', speedBps:14784921, etaSeconds:372, destination:{conferenceTag:'toronto', conferenceName:'Bitcoin++ Toronto 2026', day:'Day 1',room:'Main Stage'}, addedAt:new Date().toISOString() },
    { id:'2', name:'A002_C003_0722CD.mov', size:12884901888, uploaded:4294967296, status:'queued', speedBps:0, etaSeconds:0, destination:{conferenceTag:'toronto', conferenceName:'Bitcoin++ Toronto 2026', day:'Day 1',room:'Main Stage'}, addedAt:new Date().toISOString() },
    { id:'3', name:'A003_C002_0722EF.mov', size:9663676416, uploaded:9663676416, status:'complete', speedBps:0, etaSeconds:0, destination:{conferenceTag:'toronto', conferenceName:'Bitcoin++ Toronto 2026', day:'Day 1',room:'Talks Stage'}, addedAt:new Date().toISOString() }
  ], online:true, running:true, currentSpeedBps:14784921, averageSpeedBps:13192151, sessionUploaded:19112604467, totalBytes:41339060224, uploadedBytes:27246198784, pendingBytes:14092861440
}

const backend = () => window.go?.main?.App
export async function call(name, ...args) { const fn = backend()?.[name]; if (fn) return fn(...args); if (name==='Conferences') return [{id:'preview-toronto',tag:'toronto',description:'Bitcoin++ Toronto 2026 (preview)',location:'Toronto, Canada',starts_at:'2026-07-22T09:00:00-04:00',ends_at:'2026-07-24T17:00:00-04:00'}, {id:'preview-berlin',tag:'berlin',description:'Bitcoin++ Berlin (preview)',location:'Berlin, Germany'}]; if (name==='UploadContext') return {days:[{day_number:1,venues:['Main Stage','Talks Stage']},{day_number:2,venues:['Main Stage']}],hasHackathon:args[0]==='toronto'}; if (name==='Snapshot') return demo; if (name==='Settings') return {apiBaseUrl:'https://btcpp.dev',endpoint:'https://nyc3.digitaloceanspaces.com',region:'nyc3',bucket:'btcpp',accessKey:'',secretSet:true,partSizeMb:64,autoStart:true}; return demo }
export function onSnapshot(cb) { if (window.runtime?.EventsOn) return window.runtime.EventsOn('upload:snapshot', cb); return () => {} }
export function onFileDrop(cb) { if (window.runtime?.OnFileDrop) { window.runtime.OnFileDrop((_x,_y,paths)=>cb(paths), false); return () => window.runtime.OnFileDropOff?.() } return () => {} }
export const isDesktop = () => !!backend()
