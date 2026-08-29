import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { OAUTH_PROVIDERS, type OAuthProvider } from '../projectSchema'
import { providerLabels } from '../OAuthProviderFields'

export type AuthenticationMethod = { kind: 'oauth'; provider: OAuthProvider }

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
  const providers = OAUTH_PROVIDERS.filter((provider) => !addedOAuth.includes(provider))
  const select = (provider: OAuthProvider) => {
    onSelect({ kind: 'oauth', provider })
    onOpenChange(false)
  }

  return <DropdownMenu open={open} onOpenChange={onOpenChange}>
    <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Add authentication provider"><Plus /></Button>} />
    <DropdownMenuContent className="max-h-72 w-56" align="end">
      {providers.map((provider) => <DropdownMenuItem key={provider} onClick={() => select(provider)}>{providerLabels[provider]}</DropdownMenuItem>)}
      {!providers.length && <p className="px-2 py-1.5 text-sm text-muted-foreground">All OAuth providers are configured.</p>}
    </DropdownMenuContent>
  </DropdownMenu>
}
