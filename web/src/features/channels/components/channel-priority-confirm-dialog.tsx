/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  CheckCircle2,
  ChevronDown,
  Loader2,
  Route as RouteIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { useChannelPriorityOrder } from '../hooks/use-channel-priority-order'
import type {
  PriorityRouteChange,
  PriorityRouteTier,
} from '../lib/channel-priority-order'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

const SENSITIVE_MASK = '••••'
const EMPTY_ROUTES: PriorityRouteChange[] = []

type PriorityOrderApi = ReturnType<typeof useChannelPriorityOrder>
type ChannelNameResolver = (id: number) => string

export function ChannelPriorityConfirmDialog({
  order,
  channels,
}: {
  order: PriorityOrderApi
  channels: Channel[]
}) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const pending = order.pending
  const routes = pending?.order.routes ?? EMPTY_ROUTES
  const localNames = useMemo(
    () => new Map(channels.map((channel) => [channel.id, channel.name])),
    [channels]
  )
  const busy = order.preparing || order.saving
  const open = order.preparing || pending !== null

  const nameOf: ChannelNameResolver = (id) => {
    if (!sensitiveVisible) return SENSITIVE_MASK
    return (
      pending?.order.channel_names?.[String(id)] ??
      localNames.get(id) ??
      `#${id}`
    )
  }

  const handleOpenChange = (next: boolean) => {
    if (!next && !busy) order.decline()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Confirm priority change')}
      description={t(
        'Review the preferred and backup tiers for each group and model before saving.'
      )}
      contentHeight='55vh'
      contentClassName='sm:max-w-3xl'
      footer={<ConfirmFooter order={order} routes={routes} />}
    >
      {order.preparing ? (
        <PreparingState />
      ) : (
        <RoutePreview
          routes={routes}
          nameOf={nameOf}
          sensitiveVisible={sensitiveVisible}
        />
      )}
    </Dialog>
  )
}

function ConfirmFooter({
  order,
  routes,
}: {
  order: PriorityOrderApi
  routes: PriorityRouteChange[]
}) {
  const { t } = useTranslation()
  if (order.preparing) {
    return (
      <Button disabled>
        <Loader2 className='size-4 animate-spin' />
        {t('Preparing routing preview...')}
      </Button>
    )
  }
  if (routes.length === 0) {
    return (
      <Button variant='outline' onClick={order.decline}>
        {t('Done')}
      </Button>
    )
  }
  return (
    <>
      <Button variant='outline' onClick={order.decline} disabled={order.saving}>
        {t('Cancel')}
      </Button>
      <Button onClick={order.confirmSave} disabled={order.saving}>
        {order.saving && <Loader2 className='size-4 animate-spin' />}
        {t('Save changes')}
      </Button>
    </>
  )
}

function PreparingState() {
  const { t } = useTranslation()
  return (
    <div
      aria-live='polite'
      className='flex min-h-[240px] flex-col items-center justify-center gap-3 text-muted-foreground'
    >
      <Loader2 className='size-6 animate-spin' />
      <p className='text-sm'>{t('Preparing routing preview...')}</p>
    </div>
  )
}

function RoutePreview({
  routes,
  nameOf,
  sensitiveVisible,
}: {
  routes: PriorityRouteChange[]
  nameOf: ChannelNameResolver
  sensitiveVisible: boolean
}) {
  const { t } = useTranslation()
  const groups = useMemo(() => groupRoutes(routes), [routes])

  if (routes.length === 0) {
    return (
      <EmptyState
        icon={CheckCircle2}
        title={t('No routing changes')}
        description={t(
          'The preferred and backup channels stay the same for every group and model. Nothing will be saved.'
        )}
        className='min-h-[240px]'
      />
    )
  }
  return (
    <div className='space-y-5'>
      <p className='flex items-center gap-2 text-sm text-muted-foreground'>
        <RouteIcon className='size-4 shrink-0' />
        {t('{{count}} routing combinations affected', {
          count: routes.length,
        })}
      </p>
      {groups.map((group) => (
        <section key={group.name} className='space-y-2'>
          <GroupBadge
            group={sensitiveVisible ? group.name : undefined}
            label={sensitiveVisible ? undefined : SENSITIVE_MASK}
          />
          <div className='space-y-2'>
            {group.routes.map((route) => (
              <RouteRow
                key={`${route.group}\n${route.model}`}
                route={route}
                nameOf={nameOf}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function groupRoutes(routes: PriorityRouteChange[]) {
  const groups: { name: string; routes: PriorityRouteChange[] }[] = []
  for (const route of routes) {
    const last = groups.at(-1)
    if (last && last.name === route.group) {
      last.routes.push(route)
    } else {
      groups.push({ name: route.group, routes: [route] })
    }
  }
  return groups
}

function routeRankMap(route: PriorityRouteChange) {
  const changed = new Set<number>()
  const beforeRank = new Map<number, number>()
  route.before.forEach((tier, rank) => {
    tier.channels.forEach((id) => beforeRank.set(id, rank))
  })
  route.after.forEach((tier, rank) => {
    tier.channels.forEach((id) => {
      if (beforeRank.get(id) !== rank) changed.add(id)
    })
  })
  return changed
}

function RouteRow({
  route,
  nameOf,
}: {
  route: PriorityRouteChange
  nameOf: (id: number) => string
}) {
  const { t } = useTranslation()
  const changed = routeRankMap(route)
  return (
    <div className='space-y-2 rounded-lg border p-3'>
      <code className='block truncate text-xs font-medium' title={route.model}>
        {route.model}
      </code>
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
        <TierColumn
          heading={t('Before')}
          tiers={route.before}
          changed={changed}
          nameOf={nameOf}
        />
        <TierColumn
          heading={t('After')}
          tiers={route.after}
          changed={changed}
          nameOf={nameOf}
        />
      </div>
    </div>
  )
}

function TierColumn({
  heading,
  tiers,
  changed,
  nameOf,
}: {
  heading: string
  tiers: PriorityRouteTier[]
  changed: Set<number>
  nameOf: (id: number) => string
}) {
  return (
    <div className='space-y-1.5'>
      <div className='text-xs font-medium text-muted-foreground'>
        {heading}
      </div>
      {tiers.map((tier, index) => (
        <div key={tier.priority} className='space-y-1'>
          {index > 0 && (
            <ChevronDown className='mx-auto size-3 text-muted-foreground/70' />
          )}
          <TierBlock tier={tier} rank={index} changed={changed} nameOf={nameOf} />
        </div>
      ))}
    </div>
  )
}

function TierBlock({
  tier,
  rank,
  changed,
  nameOf,
}: {
  tier: PriorityRouteTier
  rank: number
  changed: Set<number>
  nameOf: (id: number) => string
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-1 rounded-md border border-dashed p-1.5'>
      <div className='flex items-center gap-1.5'>
        <Badge
          variant={rank === 0 ? 'default' : 'secondary'}
          className='h-5 shrink-0 px-1.5 text-[10px] leading-none'
        >
          {rank === 0 ? t('Preferred') : t('Backup {{n}}', { n: rank })}
        </Badge>
        <span className='text-[10px] text-muted-foreground tabular-nums'>
          {tier.priority}
        </span>
      </div>
      <div className='flex flex-wrap gap-1'>
        {tier.channels.map((id) => {
          const moved = changed.has(id)
          return (
            <Badge
              key={id}
              variant='outline'
              className={cn(
                'max-w-[180px] font-normal',
                moved && 'border-warning bg-warning/10 font-medium text-warning'
              )}
            >
              <span className='truncate'>{nameOf(id)}</span>
            </Badge>
          )
        })}
      </div>
    </div>
  )
}
