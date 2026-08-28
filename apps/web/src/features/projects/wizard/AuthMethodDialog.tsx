import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Input } from '@/components/ui/input'
import { OAUTH_PROVIDERS, type OAuthProvider } from '../projectSchema'
import { providerLabels } from '../OAuthProviderFields'

type Category = 'all' | 'login' | 'oauth'

export type AuthenticationMethod =
  | { kind: 'email' }
  | { kind: 'magic-link' }
  | { kind: 'phone' }
  | { kind: 'anonymous' }
  | { kind: 'smtp' }
  | { kind: 'oauth'; provider: OAuthProvider }

type Choice = { label: string; category: Exclude<Category, 'all'>; method: AuthenticationMethod }

const loginChoices: Choice[] = [
  { label: 'Email password', category: 'login', method: { kind: 'email' } },
  { label: 'Magic Link', category: 'login', method: { kind: 'magic-link' } },
  { label: 'Phone Auth', category: 'login', method: { kind: 'phone' } },
  { label: 'Anonymous sign-in', category: 'login', method: { kind: 'anonymous' } },
  { label: 'Custom SMTP', category: 'login', method: { kind: 'smtp' } },
]

export function AuthMethodDialog({
  open,
  onOpenChange,
  onSelect,
  addedOAuth,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (method: AuthenticationMethod) => void
  addedOAuth: readonly string[]
}) {
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<Category>('all')
  const choices = useMemo(() => {
    const oauthChoices: Choice[] = OAUTH_PROVIDERS
      .filter((provider) => !addedOAuth.includes(provider))
      .map((provider) => ({ label: providerLabels[provider], category: 'oauth', method: { kind: 'oauth', provider } }))
    const normalizedQuery = query.trim().toLocaleLowerCase()
    return [...loginChoices, ...oauthChoices].filter((choice) =>
      (category === 'all' || choice.category === category)
      && (!normalizedQuery || choice.label.toLocaleLowerCase().includes(normalizedQuery)),
    )
  }, [addedOAuth, category, query])

  const select = (method: AuthenticationMethod) => {
    onSelect(method)
    onOpenChange(false)
  }

  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent className="max-h-[min(42rem,calc(100vh-2rem))] max-w-lg overflow-y-auto" aria-describedby={undefined}>
      <AlertDialogHeader><AlertDialogTitle>Add authentication method</AlertDialogTitle></AlertDialogHeader>
      <Input aria-label="Search authentication methods" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search methods" autoFocus />
      <div className="flex gap-2" role="group" aria-label="Authentication method category">
        <CategoryButton active={category === 'all'} onClick={() => setCategory('all')}>All</CategoryButton>
        <CategoryButton active={category === 'login'} onClick={() => setCategory('login')}>Login methods</CategoryButton>
        <CategoryButton active={category === 'oauth'} onClick={() => setCategory('oauth')}>OAuth providers</CategoryButton>
      </div>
      <div className="space-y-1" aria-label="Authentication methods">
        {choices.map((choice) => <Button className="w-full justify-start" key={choice.label} variant="ghost" onClick={() => select(choice.method)}>{choice.label}</Button>)}
        {!choices.length && <p className="px-2 py-4 text-sm text-muted-foreground">No authentication methods found.</p>}
      </div>
      <div className="flex justify-end"><AlertDialogCancel onClick={() => onOpenChange(false)}>Cancel</AlertDialogCancel></div>
    </AlertDialogContent>
  </AlertDialog>
}

function CategoryButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return <Button type="button" size="sm" variant={active ? 'secondary' : 'ghost'} aria-pressed={active} onClick={onClick}>{children}</Button>
}
