import { useEffect, useState } from 'react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../components/ui/alert-dialog'

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

  const confirmed = mode === 'runtime' || confirmation === project.name

  return (
    <AlertDialog open={open} onOpenChange={(nextOpen) => !nextOpen && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {project.name}</AlertDialogTitle>
          <AlertDialogDescription>Choose whether to remove only the containers or also erase the project data.</AlertDialogDescription>
        </AlertDialogHeader>
        <fieldset className="delete-options">
          <label><input type="radio" name="delete-mode" checked={mode === 'runtime'} onChange={() => setMode('runtime')} /> Delete runtime only</label>
          <label><input type="radio" name="delete-mode" checked={mode === 'data'} onChange={() => setMode('data')} /> Delete runtime and data</label>
        </fieldset>
        {mode === 'data' && (
          <label>Type {project.name} to confirm
            <input aria-label={`Type ${project.name} to confirm`} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
          </label>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onClose}>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={!confirmed || busy} onClick={() => onDelete(mode, confirmation)}>Delete permanently</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
