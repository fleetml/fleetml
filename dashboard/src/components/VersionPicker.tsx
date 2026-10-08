import type { Catalog } from '../hooks/useCatalog'
import { Select } from './ui'

/** Picks a model version by name; the choice is kept in the URL (?version=). */
export default function VersionPicker({ catalog, value, onChange }: { catalog: Catalog; value: string; onChange: (id: string) => void }) {
  return (
    <div className="max-w-md mb-6">
      <Select name="version" value={value} onChange={onChange} placeholder={catalog.loading ? 'Loading…' : 'Choose a model version'}
        options={catalog.versions.map(v => ({ value: v.id, label: `${v.label} (${v.project.name}, ${v.status})` }))} />
    </div>
  )
}
