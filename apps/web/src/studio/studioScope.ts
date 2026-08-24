import type { StudioExperience, StudioProject, StudioTaskSummary } from './studioTypes';

export const ALL_EXPERIENCES='*';

export function scopedStudioProjects(projects:StudioProject[], experienceID:string, experience?:StudioExperience):StudioProject[] {
  const active=projects.filter(project=>project.status!=='archived');
  if(!experienceID||experienceID===ALL_EXPERIENCES)return active;
  return experience?active.filter(project=>experience.project_ids.includes(project.id)):[];
}

export function scopedStudioTasks(tasks:StudioTaskSummary[], experienceID:string):StudioTaskSummary[] {
  return !experienceID||experienceID===ALL_EXPERIENCES?tasks:tasks.filter(task=>task.experience_id===experienceID);
}
