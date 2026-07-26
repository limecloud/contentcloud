import { useLocation, useNavigate } from 'react-router-dom';
import { LoginView } from './LoginView';
import { RegisterView } from './RegisterView';
import { consolePath } from '../../consoleRoutes';

function safeNext(value:string|null):string {
  return value?.startsWith('/')&&!value.startsWith('//')?value:consolePath.dashboard;
}

export function LoginRoute() {
  const navigate=useNavigate();const location=useLocation();const next=safeNext(new URLSearchParams(location.search).get('next'));
  return <LoginView onSuccess={async()=>navigate(next,{replace:true})} onNavigate={path=>navigate(path)}/>;
}

export function RegisterRoute() {
  const navigate=useNavigate();const location=useLocation();const params=new URLSearchParams(location.search);
  return <RegisterView onSuccess={async()=>navigate(consolePath.dashboard,{replace:true})} onNavigate={path=>navigate(path)} initialInviteToken={params.get('invite')||undefined}/>;
}
