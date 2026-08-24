import { useSearchParams } from 'react-router-dom';
import { useStudio } from './StudioContext';
import { scopedStudioProjects } from './studioScope';
import { StudioScopeBanner } from './StudioScopeBanner';
import { GovernedKnowledgePage } from './knowledge/GovernedKnowledgePage';

export function resolveStudioProjectID(projects:{id:string;status:string}[],requestedProjectID:string|null|undefined){
  const active=projects.filter(project=>project.status!=='archived');
  return active.some(project=>project.id===requestedProjectID)?requestedProjectID||undefined:active[0]?.id;
}

export function StudioKnowledgePage(){
  const {bootstrap,selectedExperienceID}=useStudio();
  const [searchParams,setSearchParams]=useSearchParams();
  const experience=bootstrap.experiences.find(item=>item.id===selectedExperienceID);
  const projects=scopedStudioProjects(bootstrap.projects,selectedExperienceID,experience);
  const requestedProjectID=searchParams.get('project')||undefined;
  const projectID=resolveStudioProjectID(projects,requestedProjectID);
  const selectProject=(nextProjectID:string)=>{
    if(!projects.some(project=>project.id===nextProjectID))return;
    setSearchParams({project:nextProjectID},{replace:true});
  };
  return <div className="studio-knowledge-route"><StudioScopeBanner experience={experience} experienceID={selectedExperienceID} projectCount={projects.length}/><GovernedKnowledgePage projects={projects} projectID={projectID} onProject={selectProject}/></div>;
}
