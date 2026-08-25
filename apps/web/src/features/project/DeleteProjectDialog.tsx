import { useEffect, useState } from 'react'

type DeleteMode = 'runtime' | 'data'

export function DeleteProjectDialog({
  project,
  open,
  busy = false,
  onClose,
  onDelete,
}: {
  project: { id: string; name: string }
  open: boolean
  busy?: boolean
  onClose: () => void
  onDelete: (mode: DeleteMode, confirmation: string) => void
}) {
  const [mode, setMode] = useState<DeleteMode>('runtime')
  const [confirmation, setConfirmation] = useState('')

  useEffect(() => {
    if (open) {
      setMode('runtime')
      setConfirmation('')
    }
  }, [open])

  if (!open) return null
  const confirmed = mode === 'runtime' || confirmation === project.name

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="delete-project-title">
        <p className="eyebrow">Destructive action</p>
        <h2 id="delete-project-title">Delete {project.name}</h2>
        <p className="muted">Choose whether to remove only the containers or also erase the project data.</p>
        <fieldset className="delete-options">
          <label><input type="radio" name="delete-mode" checked={mode === 'runtime'} onChange={() => setMode('runtime')} /> Delete runtime only</label>
          <label><input type="radio" name="delete-mode" checked={mode === 'data'} onChange={() => setMode('data')} /> Delete runtime and data</label>
        </fieldset>
        {mode === 'data' && (
          <label>Type {project.name} to confirm
            <input aria-label={`Type ${project.name} to confirm`} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
          </label>
        )}
        <div className="dialog-actions">
          <button className="button secondary" type="button" onClick={onClose}>Cancel</button>
          <button className="button danger" type="button" disabled={!confirmed || busy} onClick={() => onDelete(mode, confirmation)}>Delete permanently</button>
        </div>
      </section>
    </div>
  )
}
