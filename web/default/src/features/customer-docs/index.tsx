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
import { AlertCircle, CheckCircle2, Copy, KeyRound, Wallet } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const approvedModels = [
  {
    category: 'GPT Text & Reasoning',
    models:
      'gpt-5.4, gpt-5.4-mini, gpt-5.5, gpt-5.6-luna, gpt-5.6-terra, gpt-5.6-sol',
    pricing: '30% of the matching OpenAI Standard short-context reference rate',
  },
  {
    category: 'DeepSeek',
    models: 'deepseek-v4-flash, deepseek-v4-pro',
    pricing: 'Official DeepSeek API base rate × 1.10',
  },
  {
    category: 'GPT Image',
    models: 'gpt-image-1, gpt-image-2',
    pricing: '30% of the matching OpenAI Standard image reference rate',
  },
]

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    toast.success('Copied')
    window.setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className='bg-muted/60 relative overflow-hidden rounded-lg border'>
      <Button
        variant='ghost'
        size='sm'
        className='absolute top-2 right-2 h-8 gap-1.5'
        onClick={copy}
      >
        {copied ? <CheckCircle2 /> : <Copy />}
        {copied ? 'Copied' : 'Copy'}
      </Button>
      <pre className='overflow-x-auto p-4 pt-12 text-xs leading-6 sm:text-sm'>
        <code>{code}</code>
      </pre>
    </div>
  )
}

export function CustomerDocs() {
  const baseUrl = `${window.location.origin}/v1`
  const curlExample = `export GLIMO_API_KEY='your-api-key'

curl ${baseUrl}/chat/completions \\
  -H "Authorization: Bearer $GLIMO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-5.6-sol",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`
  const pythonExample = `from openai import OpenAI

client = OpenAI(
    base_url="${baseUrl}",
    api_key="your-api-key",
)

response = client.chat.completions.create(
    model="deepseek-v4-pro",
    messages=[{"role": "user", "content": "Hello"}],
)
print(response.choices[0].message.content)`
  const nodeExample = `import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: '${baseUrl}',
  apiKey: process.env.GLIMO_API_KEY,
})

const response = await client.chat.completions.create({
  model: 'gpt-5.5',
  messages: [{ role: 'user', content: 'Hello' }],
})
console.log(response.choices[0].message.content)`

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>Customer Guide</SectionPageLayout.Title>

      <SectionPageLayout.Content>
        <div className='mx-auto max-w-5xl space-y-6 pb-12'>
          <p className='text-muted-foreground text-sm'>
            OpenAI-compatible quickstart, approved catalog, billing, security,
            and support for Glimo AI Gateway B2B customers.
          </p>
          <Alert>
            <AlertCircle />
            <AlertTitle>Best-effort Pilot</AlertTitle>
            <AlertDescription>
              The service is available only to approved B2B customers and does
              not include a fixed SLA. Keep the request ID when reporting an
              issue.
            </AlertDescription>
          </Alert>

          <Card id='quickstart'>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <KeyRound className='size-5' /> Quickstart
              </CardTitle>
            </CardHeader>
            <CardContent className='space-y-5'>
              <ol className='list-decimal space-y-2 pl-5 text-sm'>
                <li>Create a key from API Keys in the customer console.</li>
                <li>
                  Use Base URL <code className='font-semibold'>{baseUrl}</code>.
                </li>
                <li>
                  Store the key in an environment variable. Never commit it to
                  Git or expose it in browser code.
                </li>
              </ol>
              <div className='space-y-3'>
                <h3 className='font-semibold'>curl</h3>
                <CodeBlock code={curlExample} />
                <h3 className='pt-2 font-semibold'>Python</h3>
                <CodeBlock code={pythonExample} />
                <h3 className='pt-2 font-semibold'>Node.js</h3>
                <CodeBlock code={nodeExample} />
              </div>
              <p className='text-muted-foreground text-sm'>
                Use the endpoints shown in the signed-in Model Catalog. Text
                models normally use <code>/v1/chat/completions</code>; only use
                Responses or image generation/edit endpoints where the catalog
                marks them as verified.
              </p>
            </CardContent>
          </Card>

          <Card id='catalog'>
            <CardHeader>
              <CardTitle>Approved model catalog</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              {approvedModels.map((item) => (
                <div
                  key={item.category}
                  className='space-y-2 rounded-lg border p-4'
                >
                  <div className='flex flex-wrap items-center gap-2'>
                    <h3 className='font-semibold'>{item.category}</h3>
                    <Badge variant='secondary'>{item.pricing}</Badge>
                  </div>
                  <p className='font-mono text-sm leading-6'>{item.models}</p>
                </div>
              ))}
              <p className='text-muted-foreground text-sm'>
                Not included: codex-auto-review, dall-e-3, OpenAI long-context,
                Batch, Flex, Priority, and any model not shown in the signed-in
                catalog. Current effective prices in the private catalog and
                approved sales quote take precedence.
              </p>
            </CardContent>
          </Card>

          <Card id='billing'>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <Wallet className='size-5' /> Balance, payment, and refunds
              </CardTitle>
            </CardHeader>
            <CardContent className='space-y-4 text-sm'>
              <div className='grid gap-3 md:grid-cols-3'>
                <div className='rounded-lg border p-4'>
                  <h3 className='font-semibold'>Total Balance</h3>
                  <p className='text-muted-foreground mt-1'>
                    Recharge Balance plus Promotional Credit; this is the total
                    available for API use.
                  </p>
                </div>
                <div className='rounded-lg border p-4'>
                  <h3 className='font-semibold'>Recharge Balance</h3>
                  <p className='text-muted-foreground mt-1'>
                    Cash-funded balance. Unused value may be refunded after
                    manual review.
                  </p>
                </div>
                <div className='rounded-lg border p-4'>
                  <h3 className='font-semibold'>Promotional Credit</h3>
                  <p className='text-muted-foreground mt-1'>
                    Used first; non-refundable, non-transferable, and has no
                    cash value.
                  </p>
                </div>
              </div>
              <ul className='list-disc space-y-2 pl-5'>
                <li>US$1 paid adds US$1 Recharge Balance.</li>
                <li>Minimum Stripe top-up: US$20.</li>
                <li>
                  Card details are collected by Stripe-hosted Checkout and are
                  not stored by Glimo Lab.
                </li>
                <li>
                  Request a refund or company invoice through Refund & Support
                  in Wallet, or email{' '}
                  <a
                    href='mailto:contact@glimolab.com?subject=Glimo%20AI%20Gateway%20support%20request'
                    className='text-primary font-medium underline-offset-4 hover:underline'
                  >
                    contact@glimolab.com
                  </a>
                  . Refunds are reviewed against successful top-ups, prior
                  refunds, current balance, and usage history.
                </li>
                <li>
                  During the Pilot, Glimo Lab absorbs Stripe's non-refundable
                  original transaction fee for an approved refund.
                </li>
              </ul>
            </CardContent>
          </Card>

          <Card id='troubleshooting'>
            <CardHeader>
              <CardTitle>Usage, troubleshooting, and security</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4 text-sm'>
              <p>
                Usage Logs can be filtered by API key, model, date, status, and
                request ID. Send the request ID to{' '}
                <a
                  href='mailto:contact@glimolab.com?subject=Glimo%20AI%20Gateway%20support%20request'
                  className='text-primary font-medium underline-offset-4 hover:underline'
                >
                  contact@glimolab.com
                </a>
                , never the full API key.
              </p>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='rounded-lg border p-3'>
                  <strong>401</strong> — missing, invalid, expired, or deleted
                  API key.
                </div>
                <div className='rounded-lg border p-3'>
                  <strong>403</strong> — account suspended or model not in the
                  approved catalog.
                </div>
                <div className='rounded-lg border p-3'>
                  <strong>429</strong> — rate, concurrency, or balance limit
                  reached.
                </div>
                <div className='rounded-lg border p-3'>
                  <strong>5xx</strong> — temporary gateway or upstream issue;
                  retry only when the request is safe to repeat.
                </div>
              </div>
              <p>
                Use separate keys for separate systems, rotate them regularly,
                delete exposed keys immediately, and enable 2FA or a Passkey on
                the account.
              </p>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
