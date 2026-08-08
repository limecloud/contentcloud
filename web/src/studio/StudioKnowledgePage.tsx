import { useSearchParams } from 'react-router-dom';
import { useStudio } from './StudioContext';
import { GovernedKnowledgePage } from './knowledge/GovernedKnowledgePage';

export function resolveStudioProjectID(projects:{id:string;status:string}[],requestedProjectID:string|null|undefined){
  const active=projects.filter(project=>project.status!=='archived');
  return active.some(project=>project.id===requestedProjectID)?requestedProjectID||undefined:active[0]?.id;
}

export function StudioKnowledgePage(){
  const {bootstrap}=useStudio();
  const [searchParams,setSearchParams]=useSearchParams();
  const projects=bootstrap.projects.filter(project=>project.status!=='archived');
  const requestedProjectID=searchParams.get('project')||undefined;
  const projectID=resolveStudioProjectID(projects,requestedProjectID);
  const selectProject=(nextProjectID:string)=>{
    if(!projects.some(project=>project.id===nextProjectID))return;
    setSearchParams({project:nextProjectID},{replace:true});
  };
  return <GovernedKnowledgePage projects={projects} projectID={projectID} onProject={selectProject}/>;
}
