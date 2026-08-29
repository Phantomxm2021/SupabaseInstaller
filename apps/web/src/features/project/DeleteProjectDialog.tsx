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
import { Field, FieldLabel } from '../../components/ui/field'
import { Input } from '../../components/ui/input'
import { RadioGroup, RadioGroupItem } from '../../components/ui/radio-group'

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
          <AlertDialogDescription>Choose whether to remove only the containers or also erase the server data.</AlertDialogDescription>
        </AlertDialogHeader>
        <RadioGroup aria-label="Delete mode" name="delete-mode" value={mode} onValueChange={(value) => setMode(value as DeleteMode)} className="gap-2">
          <Field orientation="horizontal" className="items-center rounded-lg border p-3"><RadioGroupItem id="delete-runtime" value="runtime" /><FieldLabel htmlFor="delete-runtime">Delete runtime only</FieldLabel></Field>
          <Field orientation="horizontal" className="items-center rounded-lg border p-3"><RadioGroupItem id="delete-data" value="data" /><FieldLabel htmlFor="delete-data">Delete runtime and data</FieldLabel></Field>
        </RadioGroup>
        {mode === 'data' && (
          <Field><FieldLabel htmlFor="delete-confirmation">Type {project.name} to confirm</FieldLabel><Input id="delete-confirmation" aria-label={`Type ${project.name} to confirm`} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" /></Field>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onClose}>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={!confirmed || busy} onClick={() => { onClose(); onDelete(mode, confirmation) }}>Delete permanently</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
