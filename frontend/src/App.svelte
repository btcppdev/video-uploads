<script>
  import { onMount } from 'svelte'
  import { call, onSnapshot, onFileDrop, isDesktop } from './lib/bridge.js'
  import { icon } from './lib/icons.js'

  let snapshot = { revision:0, items:[], online:true, currentSpeedBps:0, averageSpeedBps:0, sessionUploaded:0, totalBytes:0, uploadedBytes:0, pendingBytes:0 }
  let tab = 'queue', dragging = false, settingsOpen = false, settings = {}, toast = ''
  let destination = { conferenceId:'toronto', conferenceTag:'toronto', conferenceName:'Bitcoin++ Toronto 2026', day:'Day 1', room:'Main Stage' }
  const events = [
    {id:'toronto',tag:'toronto',name:'Bitcoin++ Toronto 2026',meta:'July 22–24 · The Great Hall, Toronto'}
  ]
  const rooms = ['Main Stage','Talks Stage']
  const days = ['Day 1','Day 2','Day 3']

  onMount(() => {
    const off = onSnapshot(applySnapshot)
    const dropOff = onFileDrop(addPaths)
    call('Snapshot').then(applySnapshot)
    return () => { off?.(); dropOff?.() }
  })
  $: pending = snapshot.items.filter(i => !['complete','duplicate'].includes(i.status)).length
  $: completed = snapshot.items.filter(i => ['complete','duplicate'].includes(i.status)).length
  $: overall = snapshot.totalBytes ? snapshot.uploadedBytes / snapshot.totalBytes * 100 : 0
  $: visible = tab === 'complete' ? snapshot.items.filter(i=>['complete','duplicate'].includes(i.status)) : snapshot.items.filter(i=>!['complete','duplicate'].includes(i.status))

  function formatBytes(n=0) { if (!n) return '0 B'; const u=['B','KB','MB','GB','TB']; const i=Math.min(Math.floor(Math.log(n)/Math.log(1024)),4); return `${(n/1024**i).toFixed(i>2?2:i?1:0)} ${u[i]}` }
  function formatSpeed(n=0) { return n ? `${formatBytes(n)}/s` : '—' }
  function formatTime(s=0) { if (!s) return '—'; const h=Math.floor(s/3600),m=Math.floor((s%3600)/60); return h?`${h}h ${m}m`:`${Math.max(1,m)} min` }
  function dayPath(day) { return day.toLowerCase().replaceAll(' ','') }
  function roomPath(room) { return {'Main Stage':'main','Talks Stage':'talks'}[room] || room.toLowerCase().replaceAll(' ','-') }
  function selectTab(next) { tab = next }
  function applySnapshot(next) { if (next && (next.revision ?? 0) > (snapshot.revision ?? 0)) snapshot = next }
  async function addPaths(paths) { if (!paths?.length) return; try { applySnapshot(await call('AddFiles', paths, destination)); toast=`Added ${paths.length} file${paths.length>1?'s':''} to the queue`; setTimeout(()=>toast='',2800) } catch(e){ toast=String(e) } }
  async function chooseFiles(){ applySnapshot(await call('SelectVideoFiles', destination)) }
  async function action(name,id){ applySnapshot(await call(name,id)) }
  async function toggleAll(){ applySnapshot(await call(snapshot.running?'PauseAll':'ResumeAll')) }
  async function clearCompleted(){
    tab = 'queue'
    snapshot = {...snapshot, items:snapshot.items.filter(i => !['complete','duplicate'].includes(i.status))}
    try { applySnapshot(await call('ClearCompleted')) }
    catch(e) { toast=`Could not clear completed uploads: ${e}`; setTimeout(()=>toast='',3200) }
  }
  function dragover(e){e.preventDefault();dragging=true}
  function drop(e){e.preventDefault();dragging=false; const paths=[...e.dataTransfer.files].map(f=>f.path).filter(Boolean); if(paths.length)addPaths(paths); else {toast='Use “Choose files” in browser preview mode';setTimeout(()=>toast='',2600)} }
  async function openSettings(){ settings=await call('Settings');settingsOpen=true }
  async function saveSettings(){ await call('SaveSettings',settings);settingsOpen=false;toast='Connection settings saved';setTimeout(()=>toast='',2400) }
  function setEvent(e){const found=events.find(x=>x.id===e.currentTarget.value);if(found)destination={...destination,conferenceId:found.id,conferenceTag:found.tag,conferenceName:found.name}}
</script>

<svelte:head><title>bitcoin++ videos</title></svelte:head>

<div class="shell" role="application" ondragover={dragover} ondragleave={()=>dragging=false} ondrop={drop}>
  <header class="topbar">
    <div class="brand"><div class="brand-mark">✦<span>✦</span></div><div><strong>bitcoin++ videos</strong><small>FIELD KIT</small></div></div>
    <div class:offline={!snapshot.online} class="network"><span class="dot"></span>{snapshot.online?'Network stable':'Reconnecting'} <i>·</i> {formatSpeed(snapshot.currentSpeedBps)}</div>
    <button class="icon-button" aria-label="Open settings" onclick={openSettings}><svg viewBox="0 0 24 24">{@html icon('settings')}</svg></button>
  </header>

  <main>
    <section class="intro">
      <div><p class="eyebrow">FIELD UPLOAD / ROUGH MIXES</p><h1>Get the footage<br><em>off the card.</em></h1><p class="lede">Drop recordings here. We’ll keep them moving—even when the venue Wi-Fi doesn’t.</p></div>
      <div class="destination-card">
        <div class="card-label"><span>UPLOAD DESTINATION</span><span class="auto"><i></i>TORONTO FIELD DEFAULT</span></div>
        <label>Event<select value={destination.conferenceId} onchange={setEvent}>{#each events as e}<option value={e.id}>{e.name}</option>{/each}</select><small>{events.find(e=>e.id===destination.conferenceId)?.meta}</small></label>
        <div class="split"><label>Day<select bind:value={destination.day}>{#each days as d}<option>{d}</option>{/each}</select></label><label>Room<select bind:value={destination.room}>{#each rooms as r}<option>{r}</option>{/each}</select></label></div>
        <div class="path">SPACES / <b>{destination.conferenceTag}</b> / recordings / raw / {dayPath(destination.day)} / {roomPath(destination.room)}</div>
      </div>
    </section>

    <section class:active={dragging} class="dropzone">
      <div class="drop-icon"><svg viewBox="0 0 24 24">{@html icon('upload')}</svg></div>
      <div><h2>{dragging?'Drop to add recordings':'Drop video files here'}</h2><p>MOV, MP4, MXF, MTS and more · Files stay on the card while uploading</p></div>
      <button class="choose" onclick={chooseFiles}>Choose files</button>
    </section>

    <section class="metrics">
      <div><span>NOW UPLOADING</span><strong>{formatSpeed(snapshot.currentSpeedBps)}</strong><small><i class="live"></i> Live</small></div>
      <div><span>SESSION AVERAGE</span><strong>{formatSpeed(snapshot.averageSpeedBps)}</strong><small>Since app opened</small></div>
      <div><span>REMAINING</span><strong>{formatBytes(snapshot.pendingBytes)}</strong><small>{formatTime(snapshot.currentSpeedBps ? snapshot.pendingBytes/snapshot.currentSpeedBps : 0)} at current speed</small></div>
      <div><span>SESSION UPLOADED</span><strong>{formatBytes(snapshot.sessionUploaded)}</strong><small>{completed} file{completed!==1?'s':''} completed</small></div>
    </section>

    <section class="queue-section">
      <div class="queue-header">
        <div class="tabs" role="tablist" aria-label="Uploads">
          <button type="button" role="tab" data-testid="queue-tab" class:active={tab==='queue'} aria-selected={tab==='queue'} onclick={()=>selectTab('queue')}>Upload queue <span>{pending}</span></button>
          <button type="button" role="tab" data-testid="completed-tab" class:active={tab==='complete'} aria-selected={tab==='complete'} onclick={()=>selectTab('complete')}>Completed <span>{completed}</span></button>
        </div>
        {#if tab==='complete'}<button class="pause-all" onclick={clearCompleted} disabled={!completed}><svg viewBox="0 0 24 24">{@html icon('trash')}</svg>Clear all</button>{:else}<button class="pause-all" onclick={toggleAll}><svg viewBox="0 0 24 24">{@html icon(snapshot.running?'pause':'play')}</svg>{snapshot.running?'Pause all':'Resume all'}</button>{/if}
      </div>
      <div class="overall"><div><span>Overall progress</span><b>{formatBytes(snapshot.uploadedBytes)} of {formatBytes(snapshot.totalBytes)}</b></div><div class="bar"><i style={`width:${overall}%`}></i></div><strong>{Math.round(overall)}%</strong></div>
      <div class="file-list">
        {#each visible as item (item.id)}
          <article class:done={['complete','duplicate'].includes(item.status)} class:error={item.status==='error'}>
            <div class="file-icon"><svg viewBox="0 0 24 24">{@html icon(['complete','duplicate'].includes(item.status)?'check':'film')}</svg></div>
            <div class="file-main"><div class="file-title"><strong>{item.name}</strong><span>{item.destination.room} · {item.destination.day}</span></div><div class="file-progress"><div class="bar"><i style={`width:${item.status==='hashing'?(item.size?item.hashProgress/item.size*100:0):(item.size?item.uploaded/item.size*100:0)}%`}></i></div><span>{Math.round(item.status==='hashing'?(item.size?item.hashProgress/item.size*100:0):(item.size?item.uploaded/item.size*100:0))}%</span></div><small>{item.error || (item.status==='hashing' ? `Fingerprinting ${formatBytes(item.hashProgress)} of ${formatBytes(item.size)}` : `${formatBytes(item.uploaded)} of ${formatBytes(item.size)}`)}{#if item.sha256} · SHA-256 {item.sha256.slice(0,12)}…{/if}</small></div>
            <div class="file-stat">{#if item.status==='uploading'}<strong>{formatSpeed(item.speedBps)}</strong><span>{formatTime(item.etaSeconds)} left</span>{:else if item.status==='hashing'}<strong>Checking</strong><span>SHA-256</span>{:else if item.status==='duplicate'}<strong>Duplicate</strong><span>Skipped safely</span>{:else if item.status==='complete'}<strong>Uploaded</strong><span>{formatBytes(item.size)}</span>{:else}<strong>{item.status==='waiting'?'Reconnecting':item.status[0].toUpperCase()+item.status.slice(1)}</strong><span>{formatBytes(item.size-item.uploaded)} left</span>{/if}</div>
            {#if !['complete','duplicate'].includes(item.status)}<button class="row-action" aria-label={['uploading','hashing'].includes(item.status)?'Pause':'Resume'} onclick={()=>action(['uploading','hashing'].includes(item.status)?'Pause':'Resume',item.id)}><svg viewBox="0 0 24 24">{@html icon(['uploading','hashing'].includes(item.status)?'pause':'play')}</svg></button>{/if}
            <button class="row-action subtle" aria-label="Remove file" onclick={()=>action('Remove',item.id)}><svg viewBox="0 0 24 24">{@html icon('trash')}</svg></button>
          </article>
        {:else}<div class="empty"><svg viewBox="0 0 24 24">{@html icon('film')}</svg><h3>No files here yet</h3><p>Drop a card’s recordings above to get started.</p></div>{/each}
      </div>
    </section>
  </main>

  <footer><div><span class:offline={!snapshot.online} class="status-dot"></span><strong>{snapshot.online?'Protected against interruptions':'Offline — uploads will resume automatically'}</strong><span>Multipart uploads resume from the last completed chunk.</span></div><span>{isDesktop()?'DESKTOP MODE':'BROWSER PREVIEW'} · v0.1.0</span></footer>

  {#if dragging}<div class="drop-overlay"><svg viewBox="0 0 24 24">{@html icon('upload')}</svg><strong>Drop recordings anywhere</strong></div>{/if}
  {#if toast}<div class="toast">{toast}</div>{/if}
  {#if settingsOpen}<div class="modal-backdrop" role="presentation" onclick={(e)=>e.target===e.currentTarget&&(settingsOpen=false)}><form class="modal" onsubmit={(e)=>{e.preventDefault();saveSettings()}}><div class="modal-head"><div><p class="eyebrow">CONNECTION</p><h2>Uploader settings</h2></div><button type="button" class="icon-button" aria-label="Close settings" onclick={()=>settingsOpen=false}><svg viewBox="0 0 24 24">{@html icon('close')}</svg></button></div><label>Bitcoin++ API base URL<input bind:value={settings.apiBaseUrl} placeholder="https://btcpp.dev" /></label><div class="split"><label>Spaces endpoint<input bind:value={settings.endpoint}/></label><label>Region<input bind:value={settings.region}/></label></div><label>Bucket<input bind:value={settings.bucket}/></label><label>Access key<input bind:value={settings.accessKey}/></label><label>Secret key<input type="password" bind:value={settings.secretKey} placeholder={settings.secretSet?'Stored securely — leave blank to keep':'Required'} /></label><div class="modal-note">Credentials are stored in your operating system’s secure keychain. For production, the Bitcoin++ API should issue short-lived upload credentials.</div><div class="modal-actions"><button type="button" onclick={()=>settingsOpen=false}>Cancel</button><button class="primary" type="submit">Save settings</button></div></form></div>{/if}
</div>
