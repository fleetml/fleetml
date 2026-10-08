import { useCallback, useEffect, useState } from 'react'
import { getModels, getProjects, getVersions } from '../api/client'
import type { Model, ModelVersion, Project } from '../types/api'

export interface CatalogVersion extends ModelVersion {
  model: Model
  project: Project
  label: string // "iris 2.0.0"
}

export interface Catalog {
  projects: Project[]
  models: Model[]
  versions: CatalogVersion[]
  byVersionId: Record<string, CatalogVersion>
  loading: boolean
  reload: () => void
}

/** Loads projects, their models and every version, for pickers and readable names. */
export function useCatalog(): Catalog {
  const [state, setState] = useState<Omit<Catalog, 'reload'>>({ projects: [], models: [], versions: [], byVersionId: {}, loading: true })
  const load = useCallback(async () => {
    try {
      const projects = (await getProjects()) || []
      const models = (await Promise.all(projects.map(p => getModels(p.id).then(m => m || [])))).flat()
      const versions: CatalogVersion[] = (await Promise.all(models.map(async m => {
        const project = projects.find(p => p.id === m.project_id)!
        return ((await getVersions(m.id)) || []).map(v => ({ ...v, model: m, project, label: `${m.name} ${v.version}` }))
      }))).flat()
      setState({ projects, models, versions, byVersionId: Object.fromEntries(versions.map(v => [v.id, v])), loading: false })
    } catch {
      setState(s => ({ ...s, loading: false }))
    }
  }, [])
  useEffect(() => { load() }, [load])
  return { ...state, reload: load }
}
