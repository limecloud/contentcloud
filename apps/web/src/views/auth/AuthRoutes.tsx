import { useLocation, useNavigate } from 'react-router-dom';
import { LoginView } from './LoginView';
import { RegisterView } from './RegisterView';
import { safeReturnPath } from './returnPath';

export function LoginRoute() {
  const navigate=useNavigate();const location=useLocation();const next=safeReturnPath(new URLSearchParams(location.search).get('next'));
  return <LoginView onSuccess={async()=>navigate(next,{replace:true})} onNavigate={path=>navigate(path)} registerPath={`/register?next=${encodeURIComponent(next)}`}/>;
}

export function RegisterRoute() {
  const navigate=useNavigate();const location=useLocation();const params=new URLSearchParams(location.search);
  const next=safeReturnPath(params.get('next'));
  return <RegisterView onSuccess={async()=>navigate(next,{replace:true})} onNavigate={path=>navigate(path)} initialInviteToken={params.get('invite')||undefined} loginPath={`/login?next=${encodeURIComponent(next)}`}/>;
}
