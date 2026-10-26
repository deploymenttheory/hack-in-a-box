import { useState, type ChangeEvent, type FocusEvent } from 'react'
import { Field, Input, Textarea } from './components/ui/fields'
import { Icon } from './components/ui/icon'

export type GitHubSetupValues = { appID: string; slug: string; orgID: string; organization: string; template: string; privateKey: string }
type Name = keyof GitHubSetupValues
const fields: { name: Name; label: string; help: string }[] = [
  { name: 'appID', label: 'GitHub App numeric ID', help: 'Use the App ID from GitHub App settings, rather than the Client ID.' },
  { name: 'slug', label: 'GitHub App slug', help: 'The last part of github.com/apps/your-app-name.' },
  { name: 'orgID', label: 'GitHub organisation numeric ID', help: 'The numeric GitHub ID of the organisation that will own team repositories.' },
  { name: 'organization', label: 'GitHub organisation', help: 'The organisation name, without github.com or a slash.' },
  { name: 'template', label: 'Template repository name', help: 'The repository name only, within this organisation.' },
  { name: 'privateKey', label: 'App private key (PEM)', help: 'Paste the complete PEM file. It stays in this editor until confirmation and is never saved in browser storage or shown in the review.' },
]
export function gitHubSetupErrors(values: GitHubSetupValues): Partial<Record<Name, string>> {
  const errors: Partial<Record<Name, string>> = {}
  for (const { name, label } of fields) {
    const value = values[name].trim()
    if (!value) errors[name] = `Enter ${label.toLowerCase()}.`
    else if (name === 'appID' || name === 'orgID') {
      if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)) || Number(value) <= 0) errors[name] = 'Enter a positive whole-number ID, using digits only.'
    } else if (name !== 'privateKey' && !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value)) errors[name] = 'Use a name of up to 100 characters: letters, numbers, dots, underscores or hyphens. Start with a letter or number.'
    else if (name === 'privateKey' && new TextEncoder().encode(values.privateKey).length > 32768) errors[name] = 'The private key must be no larger than 32 KB.'
  }
  return errors
}

// Local feedback checks input format only. Native save/provider reads remain the
// authority for the signing key, App identity, installation and permissions.
export function GitHubSetupFields({ values, onChange, attempted = false }: { values: GitHubSetupValues; onChange: (name: Name, value: string) => void; attempted?: boolean }) {
  const [touched, setTouched] = useState<Partial<Record<Name, boolean>>>({})
  const errors = gitHubSetupErrors(values)
  function blur(name: Name, event: FocusEvent<HTMLInputElement | HTMLTextAreaElement>) {
    // A submit click focuses its button before mouseup. Let submission render
    // feedback, rather than moving that button out from under the pointer.
    if (event.relatedTarget instanceof HTMLButtonElement && event.relatedTarget.type === 'submit' && event.relatedTarget.form === event.currentTarget.form) return
    setTouched(previous => ({ ...previous, [name]: true }))
  }
  return <>{fields.map(({ name, label, help }) => {
    const show = attempted || touched[name], error = show ? errors[name] : undefined
    const props = { value: values[name], required: true, onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => onChange(name, event.target.value), onBlur: (event: FocusEvent<HTMLInputElement | HTMLTextAreaElement>) => blur(name, event) }
    return <Field key={name} label={label} error={error} description={<>{help}{show && !error && <span className="fleet-setup-field-valid"><Icon name="check" />{name === 'privateKey' ? 'Key supplied; validated when saved.' : 'Format looks right; connection not yet verified.'}</span>}</>}>
      {name === 'privateKey' ? <Textarea {...props} autoComplete="off" spellCheck={false} /> : <Input {...props} inputMode={name === 'appID' || name === 'orgID' ? 'numeric' : undefined} maxLength={name === 'appID' || name === 'orgID' ? undefined : 100} />}
    </Field>
  })}</>
}
