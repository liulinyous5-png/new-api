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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance, type i18n } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { resources } from '@/i18n/config'
import { api } from '@/lib/api'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { UsageLogsProvider } from '../usage-logs-provider'

function FilterFixture() {
  const table = useReactTable({
    data: [],
    columns: [],
    getCoreRowModel: getCoreRowModel(),
  })
  return (
    <UsageLogsProvider>
      <CommonLogsFilterBar table={table} />
    </UsageLogsProvider>
  )
}

async function renderFilter(language: i18n = i18next) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url === '/api/user/self/groups' ? {} : { quota: 0, rpm: 0, tpm: 0 },
    },
  }))
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: FilterFixture,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={language}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByRole('combobox', { name: language.t('Type') })
  return router
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('marks only retired log types as deprecated while keeping historical filters selectable', async () => {
  const router = await renderFilter()
  await userEvent.click(screen.getByRole('combobox', { name: 'Type' }))
  for (const label of ['Manage', 'Login', 'Error']) {
    expect(
      within(
        screen.getByRole('option', { name: new RegExp(`^${label}`) })
      ).getByText('Deprecated')
    ).toBeVisible()
  }
  for (const label of [
    'All Types',
    'Top-up',
    'Consume',
    'System',
    'Refund',
  ]) {
    expect(
      within(screen.getByRole('option', { name: label })).queryByText(
        'Deprecated'
      )
    ).not.toBeInTheDocument()
  }
  await userEvent.click(screen.getByRole('option', { name: /^Manage/ }))
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
    'Deprecated'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ type: ['3'], page: 1 })
  )
  await userEvent.click(screen.getByRole('combobox', { name: 'Type' }))
  await userEvent.click(screen.getByRole('option', { name: /^Login/ }))
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ type: ['7'], page: 1 })
  )
})

it.each(Object.entries(resources))(
  'loads the trace log label from the real %s translation namespace',
  async (locale, resource) => {
    const language = createInstance()
    await language.init({
      lng: locale,
      fallbackLng: false,
      resources: { [locale]: resource },
    })
    expect(language.exists('Trace request')).toBe(true)
  }
)

it('selects trace requests in the existing Type filter and translates the selected option when language changes', async () => {
  const language = createInstance()
  await language.init({ lng: 'zhCN', fallbackLng: 'en', resources })
  const router = await renderFilter(language)
  expect(
    screen.queryByRole('combobox', { name: /Request category|请求分类/ })
  ).toBeNull()
  await userEvent.click(screen.getByRole('combobox', { name: '类型' }))
  await userEvent.click(screen.getByRole('option', { name: '链路请求' }))
  await userEvent.click(screen.getByRole('button', { name: '搜索' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ type: ['8'], page: 1 })
  )
  await waitFor(() =>
    expect(
      vi
        .mocked(api.get)
        .mock.calls.some(
          ([url]) =>
            String(url).includes('/stat?') &&
            new URL(String(url), 'http://localhost').searchParams.get(
              'type'
            ) === '8'
        )
    ).toBe(true)
  )
  expect(router.state.location.search.requestType).toBeUndefined()
  await act(async () => {
    await language.changeLanguage('zhTW')
  })
  expect(
    screen.getByRole('combobox', { name: language.t('Type') })
  ).toHaveTextContent('鏈路請求')
  await act(async () => {
    await language.changeLanguage('en')
  })
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent(
    'Trace request'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  await waitFor(() => expect(router.state.location.search.type).toEqual(['0']))
})
