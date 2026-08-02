const form=document.querySelector('#search-form'),input=document.querySelector('#search'),refresh=document.querySelector('#refresh'),auto=document.querySelector('#auto'),updated=document.querySelector('#updated'),warnings=document.querySelector('#warnings'),errorBox=document.querySelector('#error'),columns=document.querySelector('#columns'),edges=document.querySelector('#edges'),empty=document.querySelector('#empty'),viewport=document.querySelector('#viewport');
let timer=null,loading=false,lastUpdated=0,data=null;
const params=new URLSearchParams(location.search);input.value=params.get('q')||'';

function escapeHTML(value){const node=document.createElement('span');node.textContent=value??'';return node.innerHTML}
function status(label,kind=''){return `<span class="status ${kind}">${label}</span>`}
function relationLabel(r){return r==='mine'?'My PR':r==='review-requested'?'Review requested':'Other'}
function nodeHTML(node){
 if(node.kind==='repository'){const r=node.repository;return `<article class="node repo" data-id="${node.id}"><strong>${escapeHTML(r.nameWithOwner)}</strong><div class="branches">default: ${escapeHTML(r.defaultBranch)}</div></article>`}
 const p=node.pullRequest,reviewKind=p.reviewDecision==='APPROVED'?'ok':p.reviewDecision==='CHANGES_REQUESTED'?'bad':'warn';
 const ciKind=p.ciState==='SUCCESS'?'ok':p.ciState==='FAILURE'||p.ciState==='ERROR'?'bad':'warn';
 const conflict=p.mergeable==='CONFLICTING'?status('⚠ Conflict','bad'):'';
 const assignees=p.assignees?.length?`<div class="assignees">Assigned: ${p.assignees.map(x=>'@'+escapeHTML(x.login)).join(', ')}</div>`:'';
 return `<article class="node ${p.isDraft?'draft':'ready'} ${p.relation}" data-id="${node.id}" aria-label="${relationLabel(p.relation)}${p.isDraft?', Draft':', Ready for review'}">
  <a class="title" href="${p.url}" target="_blank" rel="noreferrer">#${p.number} ${escapeHTML(p.title)}</a>
  <div class="author">${p.author.avatarUrl?`<img class="avatar" src="${p.author.avatarUrl}" alt="">`:''}<span>@${escapeHTML(p.author.login)}</span></div>
  <div class="branches">${escapeHTML(p.baseRefName)} ← ${escapeHTML(p.headRefName)}</div>${assignees}
  <div class="badges"><span class="badge">${relationLabel(p.relation)}</span>${p.isDraft?'<span class="badge">Draft</span>':''}${p.source==='downstream'?'<span class="badge">Downstream</span>':''}</div>
  <div class="statuses">${status(`✓ Reviews ${p.reviewApproved}/${p.reviewTotal} approved`,reviewKind)}${status(`● CI ${p.ciState||'UNKNOWN'}`,ciKind)}${conflict}</div>
 </article>`
}
function render(result){
 data=result;const oldX=viewport.scrollLeft,oldY=viewport.scrollTop;columns.innerHTML='';
 const ranks=new Map();for(const node of result.nodes){if(!ranks.has(node.rank))ranks.set(node.rank,[]);ranks.get(node.rank).push(node)}
 for(const [rank,nodes] of [...ranks.entries()].sort((a,b)=>a[0]-b[0])){const col=document.createElement('section');col.className='column';col.innerHTML=`<div class="rank-title">${rank===0?'Repositories':`Stack ${rank}`}</div>`+nodes.map(nodeHTML).join('');columns.append(col)}
 empty.hidden=result.nodes.length>0;warnings.textContent=(result.warnings||[]).join(' ');requestAnimationFrame(()=>{drawEdges();viewport.scrollLeft=oldX;viewport.scrollTop=oldY})
}
function drawEdges(){if(!data)return;const canvas=document.querySelector('#canvas'),base=canvas.getBoundingClientRect();edges.innerHTML='';for(const edge of data.edges){const a=document.querySelector(`[data-id="${CSS.escape(edge.source)}"]`),b=document.querySelector(`[data-id="${CSS.escape(edge.target)}"]`);if(!a||!b)continue;const ar=a.getBoundingClientRect(),br=b.getBoundingClientRect(),x1=ar.right-base.left,y1=ar.top+ar.height/2-base.top,x2=br.left-base.left,y2=br.top+br.height/2-base.top,m=(x1+x2)/2;const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',`M${x1},${y1} C${m},${y1} ${m},${y2} ${x2},${y2}`);path.setAttribute('fill','none');path.setAttribute('stroke','var(--line)');path.setAttribute('stroke-width','2');edges.append(path)}}
async function load(){if(loading)return;loading=true;document.body.classList.add('loading');errorBox.hidden=true;try{const response=await fetch('/api/v1/graph?q='+encodeURIComponent(input.value));const body=await response.json();if(!response.ok)throw new Error(body.error||response.statusText);render(body);lastUpdated=Date.now();updated.textContent='Updated '+new Date(lastUpdated).toLocaleTimeString()}catch(error){errorBox.textContent=error.message;errorBox.hidden=false}finally{loading=false;document.body.classList.remove('loading');schedule()}}
function schedule(){clearTimeout(timer);if(!auto.checked||document.hidden||!navigator.onLine)return;const delay=Math.max(1000,300000-(Date.now()-lastUpdated));timer=setTimeout(load,delay)}
form.addEventListener('submit',event=>{event.preventDefault();const q=input.value.trim();history.replaceState(null,'',q?'?q='+encodeURIComponent(q):location.pathname);lastUpdated=0;load()});refresh.addEventListener('click',load);auto.addEventListener('change',schedule);document.addEventListener('visibilitychange',()=>{if(!document.hidden&&Date.now()-lastUpdated>=300000)load();else schedule()});window.addEventListener('online',()=>Date.now()-lastUpdated>=300000?load():schedule());window.addEventListener('offline',schedule);window.addEventListener('resize',drawEdges);
let drag=null;viewport.addEventListener('pointerdown',e=>{if(e.target.closest('a,button,input'))return;drag={x:e.clientX,y:e.clientY,l:viewport.scrollLeft,t:viewport.scrollTop};viewport.classList.add('dragging');viewport.setPointerCapture(e.pointerId)});viewport.addEventListener('pointermove',e=>{if(drag){viewport.scrollLeft=drag.l-(e.clientX-drag.x);viewport.scrollTop=drag.t-(e.clientY-drag.y)}});viewport.addEventListener('pointerup',()=>{drag=null;viewport.classList.remove('dragging')});
load();
