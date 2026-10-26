import type { ComponentProps } from 'react'
import type { VariantProps } from 'class-variance-authority'
import { Link } from '@tanstack/react-router'
import { buttonVariants } from './components/ui/button'
import { Icon, type IconName } from './components/ui/icon'
import { cn } from './lib/utils'

// Ordinary in-app navigation must pass through the router's active departure
// barriers and preserve session-only original-request metadata. The rendered
// anchor keeps its real href, keyboard and modified-click browser semantics.
export function ResultsLink({ href, variant = 'outline', size = 'default', icon, className, children, ...props }: Omit<ComponentProps<'a'>, 'href'> & { href: string; icon?: IconName } & VariantProps<typeof buttonVariants>) {
 const destination = new URL(href, window.location.origin)
 return <Link {...props} to={destination.pathname} search={Object.fromEntries(destination.searchParams)} hash={destination.hash.slice(1)} className={cn(buttonVariants({ variant, size, className }))}>{icon && <Icon name={icon} />}{children}</Link>
}
export const isResultsDestination = (href: string) => /^\/events\/[a-f0-9-]+\/(?:results(?:\/(?:prepare|ballots|mentor-advisories))?|team-feedback|teams\/[a-f0-9-]+\/feedback-report)$/i.test(href)
