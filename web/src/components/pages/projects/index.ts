export type Project = {
  id: number;
  name: string;
  displayName: string;
  path: string;
  createdAt: string;
  updatedAt: string;
  tasksCount: number;
  taskCounts: {
    total: number;
    open: number;
    in_progress: number;
    blocked: number;
    done: number;
    ready: number;
  };
  agents: string[];
};
