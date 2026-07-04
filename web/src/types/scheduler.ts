export interface SchedulerTaskConfig {
  id: string
  name: string
  description: string
  enabled: boolean
  interval: number
  last_run_at?: string
  next_run_at?: string
}

export interface SchedulerStatus {
  running: boolean
  started_at?: string
  task_count: number
  tasks: SchedulerTaskConfig[]
}
