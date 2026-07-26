import { useParams } from 'react-router-dom';
import { DeviceAuthView, PublicReviewView } from './PublicViews';

export function DeviceAuthRoute() {return <DeviceAuthView/>}
export function PublicReviewRoute() {const {token=''}=useParams();return <PublicReviewView token={token}/>}
