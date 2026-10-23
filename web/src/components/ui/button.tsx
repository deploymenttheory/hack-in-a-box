// Base UI owns button semantics; CVA maps shared semantic design tokens.
import { Button as ButtonPrimitive } from '@base-ui/react/button'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'
const buttonVariants = cva(
  'group/button inline-flex shrink-0 items-center justify-center rounded-md border border-transparent text-sm font-semibold leading-normal whitespace-normal text-center transition-colors select-none max-w-full break-words focus-visible:[outline-width:var(--focus-width)] focus-visible:[outline-offset:var(--focus-offset)] focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-60 aria-invalid:border-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*="size-"])]:size-4',
  { variants: {
    variant: {
      default: 'bg-primary text-primary-foreground hover:bg-primary hover:underline',
      outline: 'border-input bg-background text-foreground hover:bg-accent aria-expanded:bg-accent',
      secondary: 'bg-secondary text-secondary-foreground hover:bg-accent aria-expanded:bg-accent',
      ghost: 'text-foreground hover:bg-accent aria-expanded:bg-accent',
      destructive: 'border-destructive bg-background text-destructive hover:bg-background hover:underline focus-visible:outline-destructive',
      link: 'text-primary underline underline-offset-4 hover:decoration-2',
    },
    size: {
      default: 'min-h-(--control-height) gap-2 px-3 py-1.5', xs: 'min-h-(--control-height) gap-1 px-2 py-2 text-xs',
      sm: 'min-h-(--control-height) gap-1.5 px-3 py-2 text-sm', lg: 'min-h-12 gap-2 px-5 py-3 text-base',
      icon: 'size-(--control-height) p-2', 'icon-xs': 'size-(--control-height) p-3 [&_svg:not([class*="size-"])]:size-3',
      'icon-sm': 'size-(--control-height) p-3', 'icon-lg': 'size-12 p-3',
    },
  }, defaultVariants: { variant: 'default', size: 'default' } },
)
function Button({ className, variant = 'default', size = 'default', ...props }: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return <ButtonPrimitive data-slot="button" data-variant={variant} data-size={size} className={cn(buttonVariants({ variant, size, className }))} {...props} />
}
export { Button, buttonVariants }
