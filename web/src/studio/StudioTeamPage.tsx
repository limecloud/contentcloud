import { TeamView, type TeamSession } from '../views/TeamView';
import { useStudio } from './StudioContext';

export function StudioTeamPage(){
  const {bootstrap,refresh}=useStudio();
  const session:TeamSession={role:bootstrap.session.role,user:bootstrap.session.user,tenant:bootstrap.session.tenant};
  return <TeamView session={session} onChanged={refresh}/>;
}
