const form=document.querySelector('#search-form'),input=document.querySelector('#search'),scopeAuthored=document.querySelector('#scope-authored'),scopeAssigned=document.querySelector('#scope-assigned'),scopeReview=document.querySelector('#scope-review'),hideBots=document.querySelector('#hide-bots'),refresh=document.querySelector('#refresh'),auto=document.querySelector('#auto'),updated=document.querySelector('#updated'),warnings=document.querySelector('#warnings'),errorBox=document.querySelector('#error'),lanes=document.querySelector('#lanes'),edges=document.querySelector('#edges'),empty=document.querySelector('#empty'),viewport=document.querySelector('#viewport'),progress=document.querySelector('#progress');
let timer=null,loading=false,lastUpdated=0,data=null;
const inspectCache=new Map(),includedCache=new Map(),includedConcurrency=6;
const params=new URLSearchParams(location.search);input.value=params.get('q')||'';scopeAuthored.checked=!params.has('authored')||params.get('authored')==='1';scopeAssigned.checked=!params.has('assigned')||params.get('assigned')==='1';scopeReview.checked=!params.has('reviewRequested')||params.get('reviewRequested')==='1';hideBots.checked=params.get('hideBots')==='1';

function escapeHTML(value){const node=document.createElement('span');node.textContent=value??'';return node.innerHTML}
function status(label,kind=''){return `<span class="status ${kind}">${label}</span>`}
function relationLabel(r){return r==='mine'?'My PR':r==='review-requested'?'Review requested':'Other'}
function prStateIcon(isDraft){const path=isDraft?'M3.25 1A2.25 2.25 0 0 1 4 5.372v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.251 2.251 0 0 1 3.25 1Zm9.5 14a2.25 2.25 0 1 1 0-4.5 2.25 2.25 0 0 1 0 4.5ZM2.5 3.25a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0ZM3.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm9.5 0a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5ZM14 7.5a1.25 1.25 0 1 1-2.5 0 1.25 1.25 0 0 1 2.5 0Zm0-4.25a1.25 1.25 0 1 1-2.5 0 1.25 1.25 0 0 1 2.5 0Z':'M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z';const label=isDraft?'Draft pull request':'Open pull request';return `<span class="pr-state-icon ${isDraft?'draft-icon':'ready-icon'}" title="${label}" role="img" aria-label="${label}"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="${path}"></path></svg></span>`}
function chevronIcon(expanded){const path=expanded?'M12.78 5.22a.749.749 0 0 1 0 1.06l-4.25 4.25a.749.749 0 0 1-1.06 0L3.22 6.28a.749.749 0 1 1 1.06-1.06L8 8.939l3.72-3.719a.749.749 0 0 1 1.06 0Z':'M6.22 3.22a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L9.94 8 6.22 4.28a.75.75 0 0 1 0-1.06Z';return `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="${path}"></path></svg>`}
function nodeHTML(node){
 if(node.kind==='repository'){const r=node.repository,name=escapeHTML(r.nameWithOwner);return `<article class="node repo" data-id="${node.id}"><strong>${r.url?`<a class="repo-link" href="${escapeHTML(r.url)}" target="_blank" rel="noreferrer">${name}</a>`:name}</strong></article>`}
 const p=node.pullRequest,reviewKind=p.reviewDecision==='APPROVED'?'ok':p.reviewDecision==='CHANGES_REQUESTED'?'bad':'warn';
 const ciKind=p.ciState==='SUCCESS'?'ok':p.ciState==='FAILURE'||p.ciState==='ERROR'?'bad':'warn';
 const conflict=p.mergeable==='CONFLICTING'?status('⚠ Conflict','bad'):'';
 const assigneeLabel=p.assignees?.length===1?'Assignee':'Assignees';
 const assignees=p.assignees?.length?`<div class="assignees">${assigneeLabel}: ${p.assignees.map(x=>'@'+escapeHTML(x.login)).join(', ')}</div>`:'';
 return `<article class="node ${p.isDraft?'draft':'ready'} ${p.relation}" data-id="${node.id}" aria-label="${relationLabel(p.relation)}${p.isDraft?', Draft':', Ready for review'}">
  <div class="title-row">${prStateIcon(p.isDraft)}<a class="title" href="${p.url}" target="_blank" rel="noreferrer">#${p.number} ${escapeHTML(p.title)}</a></div>
  <div class="author">${p.author.avatarUrl?`<img class="avatar" src="${p.author.avatarUrl}" alt="">`:''}<span>@${escapeHTML(p.author.login)}</span></div>
  ${assignees}
  <div class="statuses">${status(`✓ Reviews ${p.reviewApproved}/${p.reviewTotal} approved`,reviewKind)}${status(`● CI ${p.ciState||'UNKNOWN'}`,ciKind)}${conflict}</div>
  ${p.includedPullRequests?.length?`<button class="included-toggle" type="button" aria-expanded="false">${chevronIcon(false)}<span>Included PRs (${p.includedPullRequests.length})</span></button>`:''}
  ${p.includedPullRequests?.length?`<div class="included" hidden><div class="included-heading">Included PRs (${p.includedPullRequests.length})</div>${p.includedPullRequests.map(pr=>pr.url?`<a href="${pr.url}" target="_blank" rel="noreferrer">#${pr.number} ${escapeHTML(pr.title)}</a>`:`<a>#${pr.number} Loading details…</a>`).join('')}</div>`:''}
 </article>`
}
function layoutLane(lane,repoNode,prs,result){
 const nodeWidth=290,horizontalGap=72,verticalGap=24,byID=new Map(prs.map(node=>[node.id,node])),ids=new Set(byID.keys()),children=new Map(),hasParent=new Set();
 for(const node of prs)children.set(node.id,[]);
 // A node can have multiple incoming edges in a DAG. Use the first as its
 // layout parent; all edges are still drawn, but every node is positioned once.
 for(const edge of result.edges){if(!ids.has(edge.source)||!ids.has(edge.target)||hasParent.has(edge.target))continue;children.get(edge.source).push(edge.target);hasParent.add(edge.target)}
 const roots=prs.filter(node=>!hasParent.has(node.id)),subtreeHeight=new Map(),visiting=new Set();
 function element(id){return lane.querySelector(`[data-id="${CSS.escape(id)}"]`)}
 function measure(id){
  if(subtreeHeight.has(id))return subtreeHeight.get(id);const el=element(id),own=el?.offsetHeight||1;if(visiting.has(id))return own;visiting.add(id);
  const childHeights=(children.get(id)||[]).map(measure),descendants=childHeights.reduce((sum,height)=>sum+height,0)+Math.max(0,childHeights.length-1)*verticalGap,height=Math.max(own,descendants);visiting.delete(id);subtreeHeight.set(id,height);return height
 }
 const positions=new Map();
 function place(id,top){
  const el=element(id),own=el?.offsetHeight||1,height=measure(id),kids=children.get(id)||[];positions.set(id,top+(height-own)/2);let childTop=kids.length===1?top+(height-measure(kids[0]))/2:top;for(const child of kids){place(child,childTop);childTop+=measure(child)+verticalGap}
 }
 let top=0;for(const root of roots){place(root.id,top);top+=measure(root.id)+verticalGap}const contentHeight=Math.max(1,top-verticalGap),repoEl=element(repoNode.id),repoHeight=repoEl?.offsetHeight||1,laneHeight=Math.max(contentHeight,repoHeight);
 if(repoEl){repoEl.style.left='0px';repoEl.style.top=`${(laneHeight-repoHeight)/2}px`}
 let maxRank=0;for(const node of prs){maxRank=Math.max(maxRank,node.rank);const el=element(node.id);if(!el)continue;el.style.left=`${node.rank*(nodeWidth+horizontalGap)}px`;el.style.top=`${positions.get(node.id)??0}px`}
 lane.style.width=`${(maxRank+1)*nodeWidth+maxRank*horizontalGap}px`;lane.style.height=`${laneHeight}px`
}
function captureViewport(){
 const bounds=viewport.getBoundingClientRect(),centerX=bounds.left+bounds.width/2,centerY=bounds.top+bounds.height/2;let anchor=null,distance=Infinity;
 for(const node of lanes.querySelectorAll('.node[data-id]')){const rect=node.getBoundingClientRect();if(rect.right<bounds.left||rect.left>bounds.right||rect.bottom<bounds.top||rect.top>bounds.bottom)continue;const value=(rect.left+rect.width/2-centerX)**2+(rect.top+rect.height/2-centerY)**2;if(value<distance){distance=value;anchor={id:node.dataset.id,left:rect.left-bounds.left,top:rect.top-bounds.top}}}
 return {left:viewport.scrollLeft,top:viewport.scrollTop,anchor}
}
function restoreViewport(state){
 viewport.scrollLeft=state.left;viewport.scrollTop=state.top;if(!state.anchor)return;const node=lanes.querySelector(`[data-id="${CSS.escape(state.anchor.id)}"]`);if(!node)return;const bounds=viewport.getBoundingClientRect(),rect=node.getBoundingClientRect();viewport.scrollLeft+=rect.left-bounds.left-state.anchor.left;viewport.scrollTop+=rect.top-bounds.top-state.anchor.top
}
function filterBotPullRequests(result){
 if(!hideBots.checked)return result;const hidden=new Set(result.nodes.filter(node=>node.kind==='pullRequest'&&node.pullRequest.isBot).map(node=>node.id));if(!hidden.size)return result;
 const visibleRepos=new Set(result.nodes.filter(node=>node.kind==='pullRequest'&&!hidden.has(node.id)).map(node=>node.pullRequest.repositoryId)),nodes=result.nodes.filter(node=>!hidden.has(node.id)&&(node.kind!=='repository'||visibleRepos.has(node.repository.id))).map(node=>({...node})),allowed=new Set(nodes.map(node=>node.id)),outgoing=new Map(),originalEdges=new Map();
 for(const edge of result.edges){if(!outgoing.has(edge.source))outgoing.set(edge.source,[]);outgoing.get(edge.source).push(edge.target);originalEdges.set(edge.source+'\0'+edge.target,edge)}
 const edges=[],seen=new Set();for(const source of allowed){const queue=[...(outgoing.get(source)||[])],visited=new Set();while(queue.length){const target=queue.shift();if(visited.has(target))continue;visited.add(target);if(hidden.has(target)){queue.push(...(outgoing.get(target)||[]));continue}if(!allowed.has(target))continue;const key=source+'\0'+target;if(!seen.has(key)){seen.add(key);edges.push(originalEdges.get(key)||{id:'filtered:'+edges.length,source,target})}}}
 const ranks=new Map(nodes.map(node=>[node.id,node.kind==='repository'?0:1]));for(let pass=0;pass<nodes.length;pass++){let changed=false;for(const edge of edges){const next=(ranks.get(edge.source)||0)+1;if(next>(ranks.get(edge.target)||0)){ranks.set(edge.target,next);changed=true}}if(!changed)break}for(const node of nodes)node.rank=ranks.get(node.id)||node.rank;return {...result,nodes,edges}
}
function render(result){
 const viewportState=captureViewport();data=result;lanes.innerHTML='';
 const repoNodes=result.nodes.filter(node=>node.kind==='repository');
 const prsByRepo=new Map();for(const node of result.nodes){if(node.kind!=='pullRequest')continue;const id=node.pullRequest.repositoryId;if(!prsByRepo.has(id))prsByRepo.set(id,[]);prsByRepo.get(id).push(node)}
 for(const repoNode of repoNodes){
  const prs=prsByRepo.get(repoNode.repository.id)||[],lane=document.createElement('section');lane.className='repo-lane';lane.innerHTML=nodeHTML(repoNode)+prs.map(nodeHTML).join('');lanes.append(lane);layoutLane(lane,repoNode,prs,result)
 }
 empty.hidden=result.nodes.length>0;warnings.textContent=(result.warnings||[]).join(' ');requestAnimationFrame(()=>{restoreViewport(viewportState);drawEdges()})
}
function applyIncluded(updates){
 if(!updates?.length)return;const byID=new Map(updates.map(update=>[update.pullRequestId,update.includedPullRequests]));for(const node of data.nodes){if(node.kind==='pullRequest'&&byID.has(node.pullRequest.id))node.pullRequest.includedPullRequests=byID.get(node.pullRequest.id)}render(data)
}
function relayout(){
 if(!data)return;const repoNodes=data.nodes.filter(node=>node.kind==='repository'),prsByRepo=new Map();for(const node of data.nodes){if(node.kind!=='pullRequest')continue;const id=node.pullRequest.repositoryId;if(!prsByRepo.has(id))prsByRepo.set(id,[]);prsByRepo.get(id).push(node)}
 repoNodes.forEach((repoNode,index)=>{const lane=lanes.children[index];if(lane)layoutLane(lane,repoNode,prsByRepo.get(repoNode.repository.id)||[],data)});requestAnimationFrame(drawEdges)
}
function drawEdges(){if(!data)return;const canvas=document.querySelector('#canvas'),base=canvas.getBoundingClientRect();edges.innerHTML='';for(const edge of data.edges){const a=document.querySelector(`[data-id="${CSS.escape(edge.source)}"]`),b=document.querySelector(`[data-id="${CSS.escape(edge.target)}"]`);if(!a||!b)continue;const ar=a.getBoundingClientRect(),br=b.getBoundingClientRect(),x1=ar.right-base.left,y1=ar.top+ar.height/2-base.top,x2=br.left-base.left,y2=br.top+br.height/2-base.top,m=(x1+x2)/2;const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',Math.abs(y1-y2)<.5?`M${x1},${y1} L${x2},${y2}`:`M${x1},${y1} C${m},${y1} ${m},${y2} ${x2},${y2}`);path.setAttribute('fill','none');path.setAttribute('stroke','var(--line)');path.setAttribute('stroke-width','2');if(edge.dashed)path.setAttribute('stroke-dasharray','6 5');edges.append(path);if(edge.label){const label=document.createElementNS('http://www.w3.org/2000/svg','text'),title=document.createElementNS('http://www.w3.org/2000/svg','title'),short=edge.label.length>22?edge.label.slice(0,19)+'…':edge.label;label.setAttribute('x',m);label.setAttribute('y',(y1+y2)/2-6);label.setAttribute('text-anchor','middle');label.setAttribute('font-size','11');label.setAttribute('fill','var(--muted)');label.setAttribute('stroke','var(--panel)');label.setAttribute('stroke-width','3');label.setAttribute('paint-order','stroke');label.textContent=short;title.textContent=edge.label;label.append(title);edges.append(label)}}}
function setLoadProgress(percent,phase='Loading pull requests'){const value=Math.max(0,Math.min(100,percent||0));progress.setAttribute('aria-valuenow',String(value));progress.querySelector('.progress-bar').style.width=value+'%';progress.querySelector('span').textContent=`${phase} · ${value}%`}
async function readGraphResponse(response,showProgress=true){
 if(!response.ok)throw new Error(response.statusText);if(!response.headers.get('content-type')?.includes('application/x-ndjson'))return response.json();
 const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',result=null;while(true){const {value,done}=await reader.read();buffer+=decoder.decode(value||new Uint8Array(),{stream:!done});const lines=buffer.split('\n');buffer=lines.pop();for(const line of lines){if(!line.trim())continue;const event=JSON.parse(line);if(showProgress&&event.type==='progress')setLoadProgress(event.percent,event.phase);if(event.type==='error')throw new Error(event.error);if(event.type==='result')result=event.result}if(done)break}if(!result)throw new Error('GitHub response ended before the graph was ready');return result
}
function includedSignature(pr){return [...new Set(pr.includedPullRequests.map(item=>item.number))].sort((a,b)=>a-b).join(',')}
async function inspectPullRequests(result){
 const nodes=result.nodes.filter(node=>node.kind==='pullRequest'),pending=[];let completed=0;
 const finish=node=>{completed++;setLoadProgress(65+Math.round(completed/Math.max(1,nodes.length)*35),'Inspecting included pull requests');return node};
 for(const node of nodes){const pr=node.pullRequest,version=pr.headCommitSha&&pr.baseCommitSha?`${pr.headCommitSha}\0${pr.baseCommitSha}`:'',cached=inspectCache.get(pr.id);if(version&&cached?.version===version){pr.includedPullRequests=cached.value;finish(node)}else pending.push({node,version})}
 let cursor=0;
 async function worker(){while(cursor<pending.length){const item=pending[cursor++],pr=item.node.pullRequest,response=await fetch('/api/v1/inspect',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:pr.id,number:pr.number,repository:pr.repository,headCommitSha:pr.headCommitSha,baseCommitSha:pr.baseCommitSha})});if(!response.ok)throw new Error(await response.text()||response.statusText);const update=await response.json();pr.includedPullRequests=update.includedPullRequests||[];if(item.version)inspectCache.set(pr.id,{version:item.version,value:pr.includedPullRequests});finish(item.node)}}
 await Promise.all(Array.from({length:Math.min(includedConcurrency,pending.length)},worker));if(!nodes.length)setLoadProgress(100,'Complete')
}
async function hydrateIncluded(pullRequests){
 const pending=[];
 for(const pr of pullRequests){const signature=includedSignature(pr),cached=includedCache.get(pr.id);if(cached?.signature===signature)applyIncluded([{pullRequestId:pr.id,includedPullRequests:cached.value}]);else pending.push({pr,signature})}
 let cursor=0;
 async function worker(){while(cursor<pending.length){const item=pending[cursor++],response=await fetch('/api/v1/included',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pullRequests:[item.pr]})}),updates=await readGraphResponse(response,false),update=updates.find(value=>value.pullRequestId===item.pr.id);if(update){includedCache.set(item.pr.id,{signature:item.signature,value:update.includedPullRequests});applyIncluded([update])}}}
 await Promise.all(Array.from({length:Math.min(includedConcurrency,pending.length)},worker))
}
function graphURL(){const query=new URLSearchParams({q:input.value,authored:scopeAuthored.checked?'1':'0',assigned:scopeAssigned.checked?'1':'0',reviewRequested:scopeReview.checked?'1':'0'});return '/api/v1/graph?'+query}
async function load(){if(loading)return;loading=true;progress.hidden=false;setLoadProgress(0);refresh.disabled=true;updated.textContent='Loading…';errorBox.hidden=true;try{const response=await fetch(graphURL()),rawBody=await readGraphResponse(response),body=filterBotPullRequests(rawBody);await inspectPullRequests(body);render(body);setLoadProgress(100,'Complete');lastUpdated=Date.now();updated.textContent='Updated '+new Date(lastUpdated).toLocaleTimeString();await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));progress.hidden=true;const pullRequests=body.nodes.filter(node=>node.kind==='pullRequest'&&node.pullRequest.includedPullRequests?.length).map(node=>({id:node.pullRequest.id,repository:node.pullRequest.repository,includedPullRequests:node.pullRequest.includedPullRequests.map(pr=>({number:pr.number}))}));await hydrateIncluded(pullRequests)}catch(error){errorBox.textContent=error.message;errorBox.hidden=false;updated.textContent=lastUpdated?'Update failed':'Load failed'}finally{loading=false;progress.hidden=true;refresh.disabled=false;schedule()}}
function schedule(){clearTimeout(timer);if(!auto.checked||document.hidden||!navigator.onLine)return;const delay=Math.max(1000,300000-(Date.now()-lastUpdated));timer=setTimeout(load,delay)}
form.addEventListener('submit',event=>{event.preventDefault();const query=new URLSearchParams();if(input.value.trim())query.set('q',input.value.trim());query.set('authored',scopeAuthored.checked?'1':'0');query.set('assigned',scopeAssigned.checked?'1':'0');query.set('reviewRequested',scopeReview.checked?'1':'0');if(hideBots.checked)query.set('hideBots','1');history.replaceState(null,'','?'+query);lastUpdated=0;load()});hideBots.addEventListener('change',()=>{lastUpdated=0;form.requestSubmit()});refresh.addEventListener('click',load);auto.addEventListener('change',schedule);document.addEventListener('visibilitychange',()=>{if(!document.hidden&&Date.now()-lastUpdated>=300000)load();else schedule()});window.addEventListener('online',()=>Date.now()-lastUpdated>=300000?load():schedule());window.addEventListener('offline',schedule);window.addEventListener('resize',drawEdges);
lanes.addEventListener('click',async event=>{
 const button=event.target.closest('.included-toggle');if(!button)return;const container=button.closest('.node').querySelector('.included');
 container.hidden=!container.hidden;button.setAttribute('aria-expanded',String(!container.hidden));button.querySelector('svg').outerHTML=chevronIcon(!container.hidden);relayout()
});
let drag=null;viewport.addEventListener('pointerdown',e=>{if(e.target.closest('a,button,input'))return;drag={x:e.clientX,y:e.clientY,l:viewport.scrollLeft,t:viewport.scrollTop};viewport.classList.add('dragging');viewport.setPointerCapture(e.pointerId)});viewport.addEventListener('pointermove',e=>{if(drag){viewport.scrollLeft=drag.l-(e.clientX-drag.x);viewport.scrollTop=drag.t-(e.clientY-drag.y)}});viewport.addEventListener('pointerup',()=>{drag=null;viewport.classList.remove('dragging')});
load();
