import { Switch as SwitchPrimitive } from './primitives/switch'

export interface SwitchProps {
  checked?: boolean
  onChange?: (checked: boolean) => void
  disabled?: boolean
  'aria-label'?: string
  className?: string
}

export function Switch({ checked, onChange, ...props }: SwitchProps) {
  return <SwitchPrimitive checked={!!checked} onCheckedChange={onChange} {...props} />
}
