import { useParams } from 'react-router-dom';
import { DocsHome, DocsPageView, DocsShell } from './DocsViews';

export function DocsRoute(){return <DocsShell/>}
export function DocsHomeRoute(){return <DocsHome/>}
export function DocsClientRoute(){const {clientID=''}=useParams();return <DocsPageView slug={`clients/${clientID}`}/>}
export function DocsContentRoute(){const {contentKind=''}=useParams();return <DocsPageView slug={`content-types/${contentKind}`}/>}
export function DocsGuideRoute(){const {contentKind='',clientID=''}=useParams();return <DocsPageView slug={`guides/${contentKind}/${clientID}`}/>}
export function DocsGenericPageRoute(){const params=useParams();return <DocsPageView slug={params['*']||''}/>}
