/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, RotateCcw, ShieldAlert } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getAdminCodexCapacity, postCodexResetCredit } from '../api'
import type { CodexCapacityInstance, CodexCapacityWindow } from '../types'

const POLL_INTERVAL_MS = 60_000

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : formatDateTimeStr(date)
}

function CapacityWindow({
  label,
  window,
}: {
  label: string
  window?: CodexCapacityWindow | null
}) {
  const remaining = Math.max(0, Math.min(100, window?.remaining_percent ?? 0))
  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between text-xs'>
        <span className='text-muted-foreground'>{label}</span>
        <span className='font-medium tabular-nums'>
          {window ? `${remaining.toFixed(1)}%` : '-'}
        </span>
      </div>
      <Progress value={remaining} className='h-2' />
      <p className='text-muted-foreground text-[11px]'>
        Reset: {formatDate(window?.reset_at)}
      </p>
    </div>
  )
}

function getInstanceStatus(instance: CodexCapacityInstance): string {
  if (instance.stale) return 'Stale'
  if (instance.allowed) return 'Available'
  return 'Limit reached'
}

function CapacityCard({
  instance,
  onUseReset,
  pending,
}: {
  instance: CodexCapacityInstance
  onUseReset: (instance: CodexCapacityInstance) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='rounded-lg border p-4'>
      <div className='mb-4 flex items-start justify-between gap-3'>
        <div>
          <h4 className='font-medium'>{instance.display_name}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Last checked')}: {formatDate(instance.checked_at)}
          </p>
        </div>
        <Badge variant={instance.stale ? 'destructive' : 'secondary'}>
          {t(getInstanceStatus(instance))}
        </Badge>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        <CapacityWindow
          label={t('5 hour window')}
          window={instance.five_hour}
        />
        <CapacityWindow label={t('7 day window')} window={instance.seven_day} />
      </div>
      <div className='mt-4 flex items-center justify-between gap-3 border-t pt-4'>
        <div className='text-sm'>
          <span className='text-muted-foreground'>
            {t('Available reset credits')}:{' '}
          </span>
          <strong>{instance.available_reset_count}</strong>
        </div>
        <Button
          type='button'
          size='sm'
          variant='destructive'
          disabled={pending || instance.available_reset_count === 0}
          onClick={() => onUseReset(instance)}
        >
          <RotateCcw data-icon='inline-start' className='size-3.5' />
          {t('Use reset')}
        </Button>
      </div>
      {instance.credits.length > 0 && (
        <div className='mt-3 space-y-2 text-xs'>
          {instance.credits.map((credit) => (
            <div
              key={`${credit.reset_type}-${credit.status}-${credit.granted_at}-${credit.expires_at}-${credit.redeemed_at}-${credit.title}`}
              className='bg-muted/40 flex items-center justify-between gap-3 rounded-md px-3 py-2'
            >
              <span className='min-w-0 truncate'>
                {credit.title || credit.reset_type}
              </span>
              <span
                className={cn(
                  'shrink-0',
                  credit.status === 'available'
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : 'text-muted-foreground'
                )}
              >
                {credit.status} · {t('Expires')}:{' '}
                {formatDate(credit.expires_at)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function CodexCapacityPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [target, setTarget] = useState<CodexCapacityInstance | null>(null)
  const capacityQuery = useQuery({
    queryKey: ['system-info', 'codex-capacity'],
    queryFn: getAdminCodexCapacity,
    staleTime: 30_000,
    retry: false,
    refetchInterval: POLL_INTERVAL_MS,
  })
  const resetMutation = useMutation({
    mutationFn: async (instance: CodexCapacityInstance) => {
      const idempotencyKey = crypto.randomUUID()
      await postCodexResetCredit(instance.id, idempotencyKey)
    },
    onSuccess: async () => {
      toast.success(t('Official reset credit used'))
      setTarget(null)
      await queryClient.invalidateQueries({
        queryKey: ['system-info', 'codex-capacity'],
      })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Reset failed'))
    },
  })
  const instances = capacityQuery.data?.capacity.instances ?? []
  let content: ReactNode
  if (capacityQuery.isLoading) {
    content = (
      <div className='space-y-3 p-4 sm:p-5'>
        <Skeleton className='h-44 w-full' />
        <Skeleton className='h-44 w-full' />
      </div>
    )
  } else if (capacityQuery.isError) {
    content = (
      <ErrorState
        title={t('We could not load Codex capacity.')}
        description={
          capacityQuery.error instanceof Error
            ? capacityQuery.error.message
            : undefined
        }
        onRetry={() => void capacityQuery.refetch()}
        className='min-h-[180px]'
      />
    )
  } else {
    content = (
      <div className='grid gap-4 p-4 sm:p-5'>
        {instances.map((instance) => (
          <CapacityCard
            key={instance.id}
            instance={instance}
            onUseReset={setTarget}
            pending={resetMutation.isPending}
          />
        ))}
      </div>
    )
  }
  return (
    <>
      <section className='bg-card overflow-hidden rounded-lg border shadow-xs'>
        <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
          <div className='flex min-w-0 items-center gap-2'>
            <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
              <ShieldAlert className='size-4' />
            </span>
            <div>
              <h3 className='text-sm font-semibold'>{t('Codex capacity')}</h3>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t(
                  'Per-account capacity and official reset credits. Refreshes automatically every 5 minutes.'
                )}
              </p>
            </div>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void capacityQuery.refetch()}
            disabled={capacityQuery.isFetching}
          >
            <RefreshCw
              data-icon='inline-start'
              className={cn(
                'size-3.5',
                capacityQuery.isFetching && 'animate-spin'
              )}
            />
            {t('Refresh')}
          </Button>
        </div>
        {content}
      </section>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('Use official reset credit')}
        desc={
          <div className='space-y-2'>
            <p>
              {t(
                'This will consume 1 official reset credit for "{{name}}" and cannot be undone.',
                { name: target?.display_name ?? '' }
              )}
            </p>
            <p className='text-destructive'>
              {t('Used reset credits cannot be restored.')}
            </p>
          </div>
        }
        destructive
        isLoading={resetMutation.isPending}
        confirmText={
          resetMutation.isPending ? t('Resetting...') : t('Use reset')
        }
        handleConfirm={() => {
          if (target) resetMutation.mutate(target)
        }}
      />
    </>
  )
}
