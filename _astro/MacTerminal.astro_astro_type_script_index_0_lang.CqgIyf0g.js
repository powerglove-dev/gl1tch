function v(t){return Array.isArray?Array.isArray(t):Ae(t)==="[object Array]"}function ve(t){if(typeof t=="string")return t;let e=t+"";return e=="0"&&1/t==-1/0?"-0":e}function be(t){return t==null?"":ve(t)}function B(t){return typeof t=="string"}function me(t){return typeof t=="number"}function Se(t){return t===!0||t===!1||_e(t)&&Ae(t)=="[object Boolean]"}function ge(t){return typeof t=="object"}function _e(t){return ge(t)&&t!==null}function y(t){return t!=null}function X(t){return!t.trim().length}function Ae(t){return t==null?t===void 0?"[object Undefined]":"[object Null]":Object.prototype.toString.call(t)}const Ie="Incorrect 'index' type",ke=t=>`Invalid value for key ${t}`,Le=t=>`Pattern length exceeds max of ${t}.`,$e=t=>`Missing ${t} property in key`,Re=t=>`Property 'weight' in key '${t}' must be a positive integer`,ae=Object.prototype.hasOwnProperty;class Oe{constructor(e){this._keys=[],this._keyMap={};let s=0;e.forEach(n=>{let r=Ce(n);this._keys.push(r),this._keyMap[r.id]=r,s+=r.weight}),this._keys.forEach(n=>{n.weight/=s})}get(e){return this._keyMap[e]}keys(){return this._keys}toJSON(){return JSON.stringify(this._keys)}}function Ce(t){let e=null,s=null,n=null,r=1,i=null;if(B(t)||v(t))n=t,e=le(t),s=q(t);else{if(!ae.call(t,"name"))throw new Error($e("name"));const c=t.name;if(n=c,ae.call(t,"weight")&&(r=t.weight,r<=0))throw new Error(Re(c));e=le(c),s=q(c),i=t.getFn}return{path:e,id:s,weight:r,src:n,getFn:i}}function le(t){return v(t)?t:t.split(".")}function q(t){return v(t)?t.join("."):t}function Ne(t,e){let s=[],n=!1;const r=(i,c,o)=>{if(y(i))if(!c[o])s.push(i);else{let u=c[o];const a=i[u];if(!y(a))return;if(o===c.length-1&&(B(a)||me(a)||Se(a)))s.push(be(a));else if(v(a)){n=!0;for(let l=0,d=a.length;l<d;l+=1)r(a[l],c,o+1)}else c.length&&r(a,c,o+1)}};return r(t,B(e)?e.split("."):e,0),n?s:s[0]}const Te={includeMatches:!1,findAllMatches:!1,minMatchCharLength:1},Pe={isCaseSensitive:!1,ignoreDiacritics:!1,includeScore:!1,keys:[],shouldSort:!0,sortFn:(t,e)=>t.score===e.score?t.idx<e.idx?-1:1:t.score<e.score?-1:1},je={location:0,threshold:.6,distance:100},We={useExtendedSearch:!1,getFn:Ne,ignoreLocation:!1,ignoreFieldNorm:!1,fieldNormWeight:1};var h={...Pe,...Te,...je,...We};const ze=/[^ ]+/g;function He(t=1,e=3){const s=new Map,n=Math.pow(10,e);return{get(r){const i=r.match(ze).length;if(s.has(i))return s.get(i);const c=1/Math.pow(i,.5*t),o=parseFloat(Math.round(c*n)/n);return s.set(i,o),o},clear(){s.clear()}}}class oe{constructor({getFn:e=h.getFn,fieldNormWeight:s=h.fieldNormWeight}={}){this.norm=He(s,3),this.getFn=e,this.isCreated=!1,this.setIndexRecords()}setSources(e=[]){this.docs=e}setIndexRecords(e=[]){this.records=e}setKeys(e=[]){this.keys=e,this._keysMap={},e.forEach((s,n)=>{this._keysMap[s.id]=n})}create(){this.isCreated||!this.docs.length||(this.isCreated=!0,B(this.docs[0])?this.docs.forEach((e,s)=>{this._addString(e,s)}):this.docs.forEach((e,s)=>{this._addObject(e,s)}),this.norm.clear())}add(e){const s=this.size();B(e)?this._addString(e,s):this._addObject(e,s)}removeAt(e){this.records.splice(e,1);for(let s=e,n=this.size();s<n;s+=1)this.records[s].i-=1}getValueForItemAtKeyId(e,s){return e[this._keysMap[s]]}size(){return this.records.length}_addString(e,s){if(!y(e)||X(e))return;let n={v:e,i:s,n:this.norm.get(e)};this.records.push(n)}_addObject(e,s){let n={i:s,$:{}};this.keys.forEach((r,i)=>{let c=r.getFn?r.getFn(e):this.getFn(e,r.path);if(y(c)){if(v(c)){let o=[];const u=[{nestedArrIndex:-1,value:c}];for(;u.length;){const{nestedArrIndex:a,value:l}=u.pop();if(y(l))if(B(l)&&!X(l)){let d={v:l,i:a,n:this.norm.get(l)};o.push(d)}else v(l)&&l.forEach((d,p)=>{u.push({nestedArrIndex:p,value:d})})}n.$[i]=o}else if(B(c)&&!X(c)){let o={v:c,n:this.norm.get(c)};n.$[i]=o}}}),this.records.push(n)}toJSON(){return{keys:this.keys,records:this.records}}}function Ee(t,e,{getFn:s=h.getFn,fieldNormWeight:n=h.fieldNormWeight}={}){const r=new oe({getFn:s,fieldNormWeight:n});return r.setKeys(t.map(Ce)),r.setSources(e),r.create(),r}function Ke(t,{getFn:e=h.getFn,fieldNormWeight:s=h.fieldNormWeight}={}){const{keys:n,records:r}=t,i=new oe({getFn:e,fieldNormWeight:s});return i.setKeys(n),i.setIndexRecords(r),i}function H(t,{errors:e=0,currentLocation:s=0,expectedLocation:n=0,distance:r=h.distance,ignoreLocation:i=h.ignoreLocation}={}){const c=e/t.length;if(i)return c;const o=Math.abs(n-s);return r?c+o/r:o?1:c}function Ge(t=[],e=h.minMatchCharLength){let s=[],n=-1,r=-1,i=0;for(let c=t.length;i<c;i+=1){let o=t[i];o&&n===-1?n=i:!o&&n!==-1&&(r=i-1,r-n+1>=e&&s.push([n,r]),n=-1)}return t[i-1]&&i-n>=e&&s.push([n,i-1]),s}const N=32;function Ue(t,e,s,{location:n=h.location,distance:r=h.distance,threshold:i=h.threshold,findAllMatches:c=h.findAllMatches,minMatchCharLength:o=h.minMatchCharLength,includeMatches:u=h.includeMatches,ignoreLocation:a=h.ignoreLocation}={}){if(e.length>N)throw new Error(Le(N));const l=e.length,d=t.length,p=Math.max(0,Math.min(n,d));let f=i,m=p;const C=o>1||u,A=C?Array(d):[];let D;for(;(D=t.indexOf(e,m))>-1;){let w=H(e,{currentLocation:D,expectedLocation:p,distance:r,ignoreLocation:a});if(f=Math.min(w,f),m=D+l,C){let b=0;for(;b<l;)A[D+b]=1,b+=1}}m=-1;let M=[],T=1,O=l+d;const Fe=1<<l-1;for(let w=0;w<l;w+=1){let b=0,S=O;for(;b<S;)H(e,{errors:w,currentLocation:p+S,expectedLocation:p,distance:r,ignoreLocation:a})<=f?b=S:O=S,S=Math.floor((O-b)/2+b);O=S;let ce=Math.max(1,p-S+1),J=c?d:Math.min(p+S,d)+l,P=Array(J+2);P[J+1]=(1<<w)-1;for(let x=J;x>=ce;x-=1){let z=x-1,ue=s[t.charAt(z)];if(C&&(A[z]=+!!ue),P[x]=(P[x+1]<<1|1)&ue,w&&(P[x]|=(M[x+1]|M[x])<<1|1|M[x+1]),P[x]&Fe&&(T=H(e,{errors:w,currentLocation:z,expectedLocation:p,distance:r,ignoreLocation:a}),T<=f)){if(f=T,m=z,m<=p)break;ce=Math.max(1,2*p-m)}}if(H(e,{errors:w+1,currentLocation:p,expectedLocation:p,distance:r,ignoreLocation:a})>f)break;M=P}const Q={isMatch:m>=0,score:Math.max(.001,T)};if(C){const w=Ge(A,o);w.length?u&&(Q.indices=w):Q.isMatch=!1}return Q}function Ye(t){let e={};for(let s=0,n=t.length;s<n;s+=1){const r=t.charAt(s);e[r]=(e[r]||0)|1<<n-s-1}return e}const U=String.prototype.normalize?(t=>t.normalize("NFD").replace(/[\u0300-\u036F\u0483-\u0489\u0591-\u05BD\u05BF\u05C1\u05C2\u05C4\u05C5\u05C7\u0610-\u061A\u064B-\u065F\u0670\u06D6-\u06DC\u06DF-\u06E4\u06E7\u06E8\u06EA-\u06ED\u0711\u0730-\u074A\u07A6-\u07B0\u07EB-\u07F3\u07FD\u0816-\u0819\u081B-\u0823\u0825-\u0827\u0829-\u082D\u0859-\u085B\u08D3-\u08E1\u08E3-\u0903\u093A-\u093C\u093E-\u094F\u0951-\u0957\u0962\u0963\u0981-\u0983\u09BC\u09BE-\u09C4\u09C7\u09C8\u09CB-\u09CD\u09D7\u09E2\u09E3\u09FE\u0A01-\u0A03\u0A3C\u0A3E-\u0A42\u0A47\u0A48\u0A4B-\u0A4D\u0A51\u0A70\u0A71\u0A75\u0A81-\u0A83\u0ABC\u0ABE-\u0AC5\u0AC7-\u0AC9\u0ACB-\u0ACD\u0AE2\u0AE3\u0AFA-\u0AFF\u0B01-\u0B03\u0B3C\u0B3E-\u0B44\u0B47\u0B48\u0B4B-\u0B4D\u0B56\u0B57\u0B62\u0B63\u0B82\u0BBE-\u0BC2\u0BC6-\u0BC8\u0BCA-\u0BCD\u0BD7\u0C00-\u0C04\u0C3E-\u0C44\u0C46-\u0C48\u0C4A-\u0C4D\u0C55\u0C56\u0C62\u0C63\u0C81-\u0C83\u0CBC\u0CBE-\u0CC4\u0CC6-\u0CC8\u0CCA-\u0CCD\u0CD5\u0CD6\u0CE2\u0CE3\u0D00-\u0D03\u0D3B\u0D3C\u0D3E-\u0D44\u0D46-\u0D48\u0D4A-\u0D4D\u0D57\u0D62\u0D63\u0D82\u0D83\u0DCA\u0DCF-\u0DD4\u0DD6\u0DD8-\u0DDF\u0DF2\u0DF3\u0E31\u0E34-\u0E3A\u0E47-\u0E4E\u0EB1\u0EB4-\u0EB9\u0EBB\u0EBC\u0EC8-\u0ECD\u0F18\u0F19\u0F35\u0F37\u0F39\u0F3E\u0F3F\u0F71-\u0F84\u0F86\u0F87\u0F8D-\u0F97\u0F99-\u0FBC\u0FC6\u102B-\u103E\u1056-\u1059\u105E-\u1060\u1062-\u1064\u1067-\u106D\u1071-\u1074\u1082-\u108D\u108F\u109A-\u109D\u135D-\u135F\u1712-\u1714\u1732-\u1734\u1752\u1753\u1772\u1773\u17B4-\u17D3\u17DD\u180B-\u180D\u1885\u1886\u18A9\u1920-\u192B\u1930-\u193B\u1A17-\u1A1B\u1A55-\u1A5E\u1A60-\u1A7C\u1A7F\u1AB0-\u1ABE\u1B00-\u1B04\u1B34-\u1B44\u1B6B-\u1B73\u1B80-\u1B82\u1BA1-\u1BAD\u1BE6-\u1BF3\u1C24-\u1C37\u1CD0-\u1CD2\u1CD4-\u1CE8\u1CED\u1CF2-\u1CF4\u1CF7-\u1CF9\u1DC0-\u1DF9\u1DFB-\u1DFF\u20D0-\u20F0\u2CEF-\u2CF1\u2D7F\u2DE0-\u2DFF\u302A-\u302F\u3099\u309A\uA66F-\uA672\uA674-\uA67D\uA69E\uA69F\uA6F0\uA6F1\uA802\uA806\uA80B\uA823-\uA827\uA880\uA881\uA8B4-\uA8C5\uA8E0-\uA8F1\uA8FF\uA926-\uA92D\uA947-\uA953\uA980-\uA983\uA9B3-\uA9C0\uA9E5\uAA29-\uAA36\uAA43\uAA4C\uAA4D\uAA7B-\uAA7D\uAAB0\uAAB2-\uAAB4\uAAB7\uAAB8\uAABE\uAABF\uAAC1\uAAEB-\uAAEF\uAAF5\uAAF6\uABE3-\uABEA\uABEC\uABED\uFB1E\uFE00-\uFE0F\uFE20-\uFE2F]/g,"")):(t=>t);class ye{constructor(e,{location:s=h.location,threshold:n=h.threshold,distance:r=h.distance,includeMatches:i=h.includeMatches,findAllMatches:c=h.findAllMatches,minMatchCharLength:o=h.minMatchCharLength,isCaseSensitive:u=h.isCaseSensitive,ignoreDiacritics:a=h.ignoreDiacritics,ignoreLocation:l=h.ignoreLocation}={}){if(this.options={location:s,threshold:n,distance:r,includeMatches:i,findAllMatches:c,minMatchCharLength:o,isCaseSensitive:u,ignoreDiacritics:a,ignoreLocation:l},e=u?e:e.toLowerCase(),e=a?U(e):e,this.pattern=e,this.chunks=[],!this.pattern.length)return;const d=(f,m)=>{this.chunks.push({pattern:f,alphabet:Ye(f),startIndex:m})},p=this.pattern.length;if(p>N){let f=0;const m=p%N,C=p-m;for(;f<C;)d(this.pattern.substr(f,N),f),f+=N;if(m){const A=p-N;d(this.pattern.substr(A),A)}}else d(this.pattern,0)}searchIn(e){const{isCaseSensitive:s,ignoreDiacritics:n,includeMatches:r}=this.options;if(e=s?e:e.toLowerCase(),e=n?U(e):e,this.pattern===e){let C={isMatch:!0,score:0};return r&&(C.indices=[[0,e.length-1]]),C}const{location:i,distance:c,threshold:o,findAllMatches:u,minMatchCharLength:a,ignoreLocation:l}=this.options;let d=[],p=0,f=!1;this.chunks.forEach(({pattern:C,alphabet:A,startIndex:D})=>{const{isMatch:M,score:T,indices:O}=Ue(e,C,A,{location:i+D,distance:c,threshold:o,findAllMatches:u,minMatchCharLength:a,includeMatches:r,ignoreLocation:l});M&&(f=!0),p+=T,M&&O&&(d=[...d,...O])});let m={isMatch:f,score:f?p/this.chunks.length:1};return f&&r&&(m.indices=d),m}}class R{constructor(e){this.pattern=e}static isMultiMatch(e){return he(e,this.multiRegex)}static isSingleMatch(e){return he(e,this.singleRegex)}search(){}}function he(t,e){const s=t.match(e);return s?s[1]:null}class Ve extends R{constructor(e){super(e)}static get type(){return"exact"}static get multiRegex(){return/^="(.*)"$/}static get singleRegex(){return/^=(.*)$/}search(e){const s=e===this.pattern;return{isMatch:s,score:s?0:1,indices:[0,this.pattern.length-1]}}}class Qe extends R{constructor(e){super(e)}static get type(){return"inverse-exact"}static get multiRegex(){return/^!"(.*)"$/}static get singleRegex(){return/^!(.*)$/}search(e){const n=e.indexOf(this.pattern)===-1;return{isMatch:n,score:n?0:1,indices:[0,e.length-1]}}}class Je extends R{constructor(e){super(e)}static get type(){return"prefix-exact"}static get multiRegex(){return/^\^"(.*)"$/}static get singleRegex(){return/^\^(.*)$/}search(e){const s=e.startsWith(this.pattern);return{isMatch:s,score:s?0:1,indices:[0,this.pattern.length-1]}}}class Xe extends R{constructor(e){super(e)}static get type(){return"inverse-prefix-exact"}static get multiRegex(){return/^!\^"(.*)"$/}static get singleRegex(){return/^!\^(.*)$/}search(e){const s=!e.startsWith(this.pattern);return{isMatch:s,score:s?0:1,indices:[0,e.length-1]}}}class Ze extends R{constructor(e){super(e)}static get type(){return"suffix-exact"}static get multiRegex(){return/^"(.*)"\$$/}static get singleRegex(){return/^(.*)\$$/}search(e){const s=e.endsWith(this.pattern);return{isMatch:s,score:s?0:1,indices:[e.length-this.pattern.length,e.length-1]}}}class qe extends R{constructor(e){super(e)}static get type(){return"inverse-suffix-exact"}static get multiRegex(){return/^!"(.*)"\$$/}static get singleRegex(){return/^!(.*)\$$/}search(e){const s=!e.endsWith(this.pattern);return{isMatch:s,score:s?0:1,indices:[0,e.length-1]}}}class we extends R{constructor(e,{location:s=h.location,threshold:n=h.threshold,distance:r=h.distance,includeMatches:i=h.includeMatches,findAllMatches:c=h.findAllMatches,minMatchCharLength:o=h.minMatchCharLength,isCaseSensitive:u=h.isCaseSensitive,ignoreDiacritics:a=h.ignoreDiacritics,ignoreLocation:l=h.ignoreLocation}={}){super(e),this._bitapSearch=new ye(e,{location:s,threshold:n,distance:r,includeMatches:i,findAllMatches:c,minMatchCharLength:o,isCaseSensitive:u,ignoreDiacritics:a,ignoreLocation:l})}static get type(){return"fuzzy"}static get multiRegex(){return/^"(.*)"$/}static get singleRegex(){return/^(.*)$/}search(e){return this._bitapSearch.searchIn(e)}}class Me extends R{constructor(e){super(e)}static get type(){return"include"}static get multiRegex(){return/^'"(.*)"$/}static get singleRegex(){return/^'(.*)$/}search(e){let s=0,n;const r=[],i=this.pattern.length;for(;(n=e.indexOf(this.pattern,s))>-1;)s=n+i,r.push([n,s-1]);const c=!!r.length;return{isMatch:c,score:c?0:1,indices:r}}}const ee=[Ve,Me,Je,Xe,qe,Ze,Qe,we],de=ee.length,et=/ +(?=(?:[^\"]*\"[^\"]*\")*[^\"]*$)/,tt="|";function st(t,e={}){return t.split(tt).map(s=>{let n=s.trim().split(et).filter(i=>i&&!!i.trim()),r=[];for(let i=0,c=n.length;i<c;i+=1){const o=n[i];let u=!1,a=-1;for(;!u&&++a<de;){const l=ee[a];let d=l.isMultiMatch(o);d&&(r.push(new l(d,e)),u=!0)}if(!u)for(a=-1;++a<de;){const l=ee[a];let d=l.isSingleMatch(o);if(d){r.push(new l(d,e));break}}}return r})}const nt=new Set([we.type,Me.type]);class rt{constructor(e,{isCaseSensitive:s=h.isCaseSensitive,ignoreDiacritics:n=h.ignoreDiacritics,includeMatches:r=h.includeMatches,minMatchCharLength:i=h.minMatchCharLength,ignoreLocation:c=h.ignoreLocation,findAllMatches:o=h.findAllMatches,location:u=h.location,threshold:a=h.threshold,distance:l=h.distance}={}){this.query=null,this.options={isCaseSensitive:s,ignoreDiacritics:n,includeMatches:r,minMatchCharLength:i,findAllMatches:o,ignoreLocation:c,location:u,threshold:a,distance:l},e=s?e:e.toLowerCase(),e=n?U(e):e,this.pattern=e,this.query=st(this.pattern,this.options)}static condition(e,s){return s.useExtendedSearch}searchIn(e){const s=this.query;if(!s)return{isMatch:!1,score:1};const{includeMatches:n,isCaseSensitive:r,ignoreDiacritics:i}=this.options;e=r?e:e.toLowerCase(),e=i?U(e):e;let c=0,o=[],u=0;for(let a=0,l=s.length;a<l;a+=1){const d=s[a];o.length=0,c=0;for(let p=0,f=d.length;p<f;p+=1){const m=d[p],{isMatch:C,indices:A,score:D}=m.search(e);if(C){if(c+=1,u+=D,n){const M=m.constructor.type;nt.has(M)?o=[...o,...A]:o.push(A)}}else{u=0,c=0,o.length=0;break}}if(c){let p={isMatch:!0,score:u/c};return n&&(p.indices=o),p}}return{isMatch:!1,score:1}}}const te=[];function it(...t){te.push(...t)}function se(t,e){for(let s=0,n=te.length;s<n;s+=1){let r=te[s];if(r.condition(t,e))return new r(t,e)}return new ye(t,e)}const Y={AND:"$and",OR:"$or"},ne={PATH:"$path",PATTERN:"$val"},re=t=>!!(t[Y.AND]||t[Y.OR]),ot=t=>!!t[ne.PATH],ct=t=>!v(t)&&ge(t)&&!re(t),fe=t=>({[Y.AND]:Object.keys(t).map(e=>({[e]:t[e]}))});function xe(t,e,{auto:s=!0}={}){const n=r=>{let i=Object.keys(r);const c=ot(r);if(!c&&i.length>1&&!re(r))return n(fe(r));if(ct(r)){const u=c?r[ne.PATH]:i[0],a=c?r[ne.PATTERN]:r[u];if(!B(a))throw new Error(ke(u));const l={keyId:q(u),pattern:a};return s&&(l.searcher=se(a,e)),l}let o={children:[],operator:i[0]};return i.forEach(u=>{const a=r[u];v(a)&&a.forEach(l=>{o.children.push(n(l))})}),o};return re(t)||(t=fe(t)),n(t)}function ut(t,{ignoreFieldNorm:e=h.ignoreFieldNorm}){t.forEach(s=>{let n=1;s.matches.forEach(({key:r,norm:i,score:c})=>{const o=r?r.weight:null;n*=Math.pow(c===0&&o?Number.EPSILON:c,(o||1)*(e?1:i))}),s.score=n})}function at(t,e){const s=t.matches;e.matches=[],y(s)&&s.forEach(n=>{if(!y(n.indices)||!n.indices.length)return;const{indices:r,value:i}=n;let c={indices:r,value:i};n.key&&(c.key=n.key.src),n.idx>-1&&(c.refIndex=n.idx),e.matches.push(c)})}function lt(t,e){e.score=t.score}function ht(t,e,{includeMatches:s=h.includeMatches,includeScore:n=h.includeScore}={}){const r=[];return s&&r.push(at),n&&r.push(lt),t.map(i=>{const{idx:c}=i,o={item:e[c],refIndex:c};return r.length&&r.forEach(u=>{u(i,o)}),o})}class W{constructor(e,s={},n){this.options={...h,...s},this.options.useExtendedSearch,this._keyStore=new Oe(this.options.keys),this.setCollection(e,n)}setCollection(e,s){if(this._docs=e,s&&!(s instanceof oe))throw new Error(Ie);this._myIndex=s||Ee(this.options.keys,this._docs,{getFn:this.options.getFn,fieldNormWeight:this.options.fieldNormWeight})}add(e){y(e)&&(this._docs.push(e),this._myIndex.add(e))}remove(e=()=>!1){const s=[];for(let n=0,r=this._docs.length;n<r;n+=1){const i=this._docs[n];e(i,n)&&(this.removeAt(n),n-=1,r-=1,s.push(i))}return s}removeAt(e){this._docs.splice(e,1),this._myIndex.removeAt(e)}getIndex(){return this._myIndex}search(e,{limit:s=-1}={}){const{includeMatches:n,includeScore:r,shouldSort:i,sortFn:c,ignoreFieldNorm:o}=this.options;let u=B(e)?B(this._docs[0])?this._searchStringList(e):this._searchObjectList(e):this._searchLogical(e);return ut(u,{ignoreFieldNorm:o}),i&&u.sort(c),me(s)&&s>-1&&(u=u.slice(0,s)),ht(u,this._docs,{includeMatches:n,includeScore:r})}_searchStringList(e){const s=se(e,this.options),{records:n}=this._myIndex,r=[];return n.forEach(({v:i,i:c,n:o})=>{if(!y(i))return;const{isMatch:u,score:a,indices:l}=s.searchIn(i);u&&r.push({item:i,idx:c,matches:[{score:a,value:i,norm:o,indices:l}]})}),r}_searchLogical(e){const s=xe(e,this.options),n=(o,u,a)=>{if(!o.children){const{keyId:d,searcher:p}=o,f=this._findMatches({key:this._keyStore.get(d),value:this._myIndex.getValueForItemAtKeyId(u,d),searcher:p});return f&&f.length?[{idx:a,item:u,matches:f}]:[]}const l=[];for(let d=0,p=o.children.length;d<p;d+=1){const f=o.children[d],m=n(f,u,a);if(m.length)l.push(...m);else if(o.operator===Y.AND)return[]}return l},r=this._myIndex.records,i={},c=[];return r.forEach(({$:o,i:u})=>{if(y(o)){let a=n(s,o,u);a.length&&(i[u]||(i[u]={idx:u,item:o,matches:[]},c.push(i[u])),a.forEach(({matches:l})=>{i[u].matches.push(...l)}))}}),c}_searchObjectList(e){const s=se(e,this.options),{keys:n,records:r}=this._myIndex,i=[];return r.forEach(({$:c,i:o})=>{if(!y(c))return;let u=[];n.forEach((a,l)=>{u.push(...this._findMatches({key:a,value:c[l],searcher:s}))}),u.length&&i.push({idx:o,item:c,matches:u})}),i}_findMatches({key:e,value:s,searcher:n}){if(!y(s))return[];let r=[];if(v(s))s.forEach(({v:i,i:c,n:o})=>{if(!y(i))return;const{isMatch:u,score:a,indices:l}=n.searchIn(i);u&&r.push({score:a,key:e,value:i,idx:c,norm:o,indices:l})});else{const{v:i,n:c}=s,{isMatch:o,score:u,indices:a}=n.searchIn(i);o&&r.push({score:u,key:e,value:i,norm:c,indices:a})}return r}}W.version="7.1.0";W.createIndex=Ee;W.parseIndex=Ke;W.config=h;W.parseQuery=xe;it(rt);let k=null,ie=null;const K=[];let _=-1;const E=document.getElementById("terminal-input"),$=document.getElementById("terminal-output");function De(){const t=document.getElementById("tui-clock");if(!t)return;const e=new Date;let s=e.getHours();const n=e.getMinutes().toString().padStart(2,"0"),r=s>=12?"pm":"am";s=s%12||12,t.textContent=`${s}:${n}${r}`}De();setInterval(De,1e4);async function dt(){try{k=await(await fetch("/glitch-db.json")).json(),ie=new W(k.qa,{keys:[{name:"question",weight:.7},{name:"keywords",weight:.3}],threshold:.4,includeScore:!0})}catch{F("output-error","Could not load response database. Refresh to retry.")}document.getElementById("mac-terminal")?.addEventListener("click",()=>{E.focus()}),E.disabled=!0,await yt(),E.disabled=!1,E.addEventListener("keydown",ft),E.focus()}function ft(t){switch(t.key){case"Enter":{const e=E.value.trim();if(E.value="",!e)return;K.unshift(e),_=-1,F("output-cmd",`>> ${e}`),pt(e);break}case"ArrowUp":t.preventDefault(),_<K.length-1&&(_++,E.value=K[_]);break;case"ArrowDown":t.preventDefault(),_>0?(_--,E.value=K[_]):(_=-1,E.value="");break;case"Tab":t.preventDefault(),wt();break;case"l":t.ctrlKey&&(t.preventDefault(),Be());break}}function pt(t){if(!k){F("output-error","Database not loaded. Refresh to retry.");return}if(t.startsWith("/")){const e=t.slice(1).toLowerCase().trim();if(e==="clear"||e==="c"){Be();return}if(e==="help"||e==="h"||e==="?"){j(k.commands.help??"Type /install, /pipelines, or /brain to get started.");return}if(e==="install"){gt();return}if(e==="screenshots"||e==="ss"){At();return}if(e==="docs"||e==="d"){window.DOCS_PANEL?window.DOCS_PANEL.open():window.location.href="/docs";return}if(e==="ecosystem"||e==="plugins"){window.ECOSYSTEM_PANEL?window.ECOSYSTEM_PANEL.open():window.location.href="/ecosystem";return}if(e==="pipelines"||e==="pipeline"){Et();return}if(k.commands[e]){j(k.commands[e]);return}F("output-error",`/${e}: command not found. Type /help for a list.`);return}if(ie){const e=ie.search(t);if(e.length>0&&(e[0].score??1)<.5){j(e[0].item.response);return}}F("output-dim","No response found for that. Try /help.")}async function mt(){const t=navigator.userAgent.toLowerCase(),e=(navigator.platform??"").toLowerCase();let s="linux",n="amd64";e.includes("mac")||t.includes("macintosh")?s="darwin":(e.includes("win")||t.includes("windows"))&&(s="windows");try{const r=navigator.userAgentData;r?.getHighEntropyValues&&(await r.getHighEntropyValues(["architecture"])).architecture==="arm"&&(n="arm64")}catch{}return n==="amd64"&&(t.includes("arm64")||t.includes("aarch64"))&&(n="arm64"),{os:s,arch:n}}async function gt(){F("output-dim","detecting platform...");let t=null;try{const a=await(await fetch("/release.json")).json();a.version&&(t=a)}catch{}const{os:e,arch:s}=await mt();if(!t||!t.version){j(`Install via Homebrew (recommended):

  brew install 8op-org/tap/glitch

Or build from source:

  go install github.com/8op-org/gl1tch/cmd/glitch@latest

Then pull a local model:

  ollama pull qwen2.5-coder:latest

Run \`glitch\`. That's it.`);return}const n=`${e}_${s}`,r=t.assets[n]??"",i=t.version,c=e==="windows";let o=`GLITCH ${i}

`;if(c){const u=t.assets.linux_amd64??"";o+=`Windows detected.

`,o+=`WSL2 (recommended — full tmux experience):
`,u&&(o+=`  curl -L "${u}" | tar xz -C /tmp/glitch-install
`,o+=`  cd /tmp/glitch-install && ./install.sh
`),o+=`
Native Windows:
`,r&&(o+=`  Download: ${r}
`,o+=`  Extract the zip and run install.sh in PowerShell.
`),o+=`
WSL2 gives you the full experience. tmux required.`}else{o+=`${e==="darwin"?s==="arm64"?"macOS — Apple Silicon":"macOS — Intel":s==="arm64"?"Linux — ARM64":"Linux — x86_64"}

`,e==="darwin"&&(o+=`  brew install 8op-org/tap/glitch

`,o+=`Or download the binary directly:

`),r&&(o+=`  curl -L "${r}" | tar xz -C /tmp/glitch-install
`,o+=`  cd /tmp/glitch-install && ./install.sh

`),o+=`Make sure ~/.local/bin is on your $PATH.

`;const a=s==="arm64"?"amd64":"arm64",l=e==="darwin"?a==="arm64"?"Apple Silicon":"Intel":a==="arm64"?"ARM64":"x86_64",d=t.assets[`${e}_${a}`]??"";d&&(o+=`Wrong arch? ${l}: ${d}`)}j(o)}let g=null,I=0,L=[];async function At(){let t=[];try{t=await(await fetch("/screenshots/index.json")).json()}catch{F("output-error","Could not load screenshots. Run tools/screenshots/take.sh first.");return}if(t.length===0){F("output-dim","No screenshots yet. Run: tools/screenshots/take.sh");return}L=t,I=0,g&&g.remove(),g=document.createElement("div"),g.className="output-line screenshot-gallery",g.setAttribute("tabindex","0"),$.appendChild(g),V(),Z(),g.addEventListener("keydown",e=>{e.key==="ArrowRight"||e.key==="l"?(e.preventDefault(),I=(I+1)%L.length,Z()):e.key==="ArrowLeft"||e.key==="h"?(e.preventDefault(),I=(I-1+L.length)%L.length,Z()):(e.key==="Escape"||e.key==="q")&&(e.preventDefault(),Ct(),E.focus())}),g.focus()}function Z(){if(!g||L.length===0)return;const t=L[I];for(;g.firstChild;)g.removeChild(g.firstChild);const e=document.createElement("div");e.className="sg-header";const s=document.createElement("span");s.className="sg-counter",s.textContent=`[${I+1}/${L.length}]`;const n=document.createElement("span");n.className="sg-caption",n.textContent=t.caption;const r=document.createElement("span");r.className="sg-keys",r.textContent="← → navigate · esc close",e.appendChild(s),e.appendChild(n),e.appendChild(r);const i=document.createElement("div");i.className="sg-img-wrap";const c=document.createElement("img");c.className="sg-img",c.setAttribute("loading","lazy"),c.src=`/screenshots/${t.file.replace(/[^a-zA-Z0-9._-]/g,"")}`,c.alt=t.caption,i.appendChild(c);const o=document.createElement("div");o.className="sg-dots",L.forEach((u,a)=>{const l=document.createElement("span");l.className=a===I?"sg-dot sg-dot-active":"sg-dot",o.appendChild(l)}),g.appendChild(e),g.appendChild(i),g.appendChild(o)}function Ct(){g&&(g.remove(),g=null)}const pe=[{name:"code-review",desc:"local model reads the diff, cloud model judges it",yaml:`name: code-review
version: "1"

steps:
  - id: read
    plugin: ollama
    model: qwen2.5-coder:latest
    prompt: |
      Summarize what changed in this diff: {{input}}
      Be specific. List files touched and intent.

  - id: review
    executor: claude
    model: claude-haiku-4-5-20251001
    needs: [read]
    prompt: |
      {{steps.read.output}}
      Flag bugs, security issues, or patterns that look wrong.`},{name:"morning-standup",desc:"summarize yesterday's commits into a standup update",yaml:`name: morning-standup
version: "1"

steps:
  - id: commits
    plugin: sh
    vars:
      cmd: "git log --oneline --since=yesterday"

  - id: standup
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: |
      Commits from yesterday:
      {{steps.commits.output}}

      Write a 3-sentence standup update.
      What was done. Any blockers. What's next.`},{name:"gh-triage",desc:"pull open issues and prioritize them by impact",yaml:`name: gh-triage
version: "1"

steps:
  - id: issues
    plugin: gh
    vars:
      args: "issue list --json number,title,labels --limit 20"

  - id: triage
    executor: claude
    model: claude-sonnet-4-6
    needs: [issues]
    prompt: |
      Open issues:
      {{steps.issues.output}}

      Rank by impact. Flag anything blocking a release.
      Output as a numbered list with one-line rationale each.`},{name:"brain-map",desc:"index the codebase architecture into brain context",yaml:`name: brain-map
version: "1"

steps:
  - id: scan
    plugin: sh
    vars:
      cmd: "find . -name '*.go' | head -30 | xargs head -5"

  - id: map
    plugin: ollama
    model: qwen2.5-coder:latest
    needs: [scan]
    prompt: |
      Source files:
      {{steps.scan.output}}

      Write a concise architecture map.
      Packages, responsibilities, key patterns.
      Start with "ARCHITECTURE:"`},{name:"doc-gen",desc:"generate godoc comments for undocumented functions",yaml:`name: doc-gen
version: "1"

steps:
  - id: read
    plugin: sh
    vars:
      cmd: "grep -n 'func ' {{input}} | head -20"

  - id: docs
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: |
      Functions in {{input}}:
      {{steps.read.output}}

      Write godoc comments for each exported function.
      Follow standard Go doc conventions.`},{name:"security-scan",desc:"scan code for common vulnerabilities",yaml:`name: security-scan
version: "1"

steps:
  - id: read
    plugin: sh
    vars:
      cmd: "cat {{input}}"

  - id: scan
    executor: claude
    model: claude-sonnet-4-6
    prompt: |
      Code:
      {{steps.read.output}}

      Check for: SQL injection, command injection, hardcoded secrets,
      insecure deserialization, path traversal, missing auth checks.
      Report findings with line references.`},{name:"changelog-draft",desc:"draft a changelog entry from recent commits",yaml:`name: changelog-draft
version: "1"

steps:
  - id: log
    plugin: sh
    vars:
      cmd: "git log --oneline v{{input}}..HEAD"

  - id: draft
    executor: claude
    model: claude-haiku-4-5-20251001
    prompt: |
      Commits since last release:
      {{steps.log.output}}

      Draft a changelog entry in Keep a Changelog format.
      Group into: Added, Changed, Fixed, Removed.`}];function Et(){const t=pe[Math.floor(Math.random()*pe.length)];j(`${t.name}  —  ${t.desc}

${t.yaml}

run it:  glitch pipeline run ${t.name}.yaml
type /pipelines again for another example.`)}function F(t,e){const s=document.createElement("div");s.className=`output-line ${t}`,s.textContent=e,$.appendChild(s),V()}let G=null;function j(t){G&&(clearTimeout(G),G=null);const e=document.createElement("div");return e.className="output-line output-response",$.appendChild(e),V(),new Promise(s=>{let n=0;function r(){if(n<t.length){e.textContent=t.slice(0,n+1),n++;const i=t[n-1];G=setTimeout(r,i===`
`||i==="."||i===":"?8:3),V()}else s()}r()})}async function yt(){F("output-dim","gl1tch ready.")}function Be(){for(;$.firstChild;)$.removeChild($.firstChild)}function wt(){if(!k||!E.value.startsWith("/"))return;const t=E.value.slice(1).toLowerCase(),s=[...Object.keys(k.commands),"screenshots","clear","docs","ecosystem","plugins"].filter(n=>n.startsWith(t));s.length===1?E.value=`/${s[0]}`:s.length>1&&F("output-dim",s.map(n=>`/${n}`).join("  "))}function V(){$.scrollTop=$.scrollHeight}function Mt(){const t=document.getElementById("hexbg");if(!t)return;const e=t.getContext("2d"),s=28,n=18;let r,i,c,o,u;function a(){return Math.floor(Math.random()*256).toString(16).padStart(2,"0").toUpperCase()}function l(){t.width=window.innerWidth,t.height=window.innerHeight}function d(){r=Math.ceil(t.width/s)+2,i=Math.ceil(t.height/n)+3,c=Array.from({length:r},()=>Math.random()*i*n),o=Array.from({length:r},()=>.25+Math.random()*.45);const f=i*4;u=Array.from({length:f},()=>Array.from({length:r},a))}function p(){e.clearRect(0,0,t.width,t.height),e.font='12px "JetBrains Mono", monospace',e.fillStyle="rgba(135, 135, 175, 0.07)";for(let f=0;f<r;f++){c[f]=(c[f]+o[f])%(u.length*n);const m=Math.floor(c[f]/n),C=c[f]%n;for(let A=0;A<=i+1;A++){const D=A*n-C,M=(m+A)%u.length;e.fillText(u[M][f],f*s,D)}}requestAnimationFrame(p)}l(),d(),window.addEventListener("resize",()=>{l(),d()}),p()}document.addEventListener("DOMContentLoaded",()=>{Mt(),dt()});
