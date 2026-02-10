<script setup lang="ts">
import type { TableColumn,  } from '@nuxt/ui'
import {api, type SuggestBlock} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {UButton} from "#components";
import {getErrorMessage} from "~~/utils/get-error-message";

useHead({
  title: 'Suggest',
})

let lastFetchController: AbortController | null = null

const records = ref<SuggestBlock[]>([])
const globalFilter = ref('')

const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

const pagination = ref({
  pageIndex: 0,
  pageSize: 12,
  total: 0,
})

const fetchData = async () => {
  try {
    if (lastFetchController) lastFetchController.abort()
    lastFetchController = new AbortController()
    const response = await api.getAllSuggestRecords({
      limit: pagination.value.pageSize,
      offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
      filter: globalFilter.value,
      active: true,
    }, lastFetchController.signal)

    records.value = response.list
    pagination.value = {
      ...pagination.value,
      total: response.total,
    }
  } catch (error) {
    const message = getErrorMessage(error)
    toast.add({
      title: 'Error',
      description: message,
      duration: 5000,
      color: 'error',
    })
    console.error('Error fetching data:', error)
  }
}
const toast = useToast()
const fetchWithLoading = createLoadingRequest(fetchData)

const changeFilter = async () => {
  pagination.value.pageIndex = 0
  await fetchWithLoading()
}

const changePage = async (page: number) => {
  pagination.value.pageIndex = page - 1
  await fetchWithLoading()
}
onMounted(fetchWithLoading)

const createDomain = async (item: SuggestBlock) => {
  try {
    await api.addSuggestToBlock(item)
    await fetchWithLoading()
    toast.add({
      title: 'Success',
      description: 'New domain was added.',
      duration: 3000,
    })
  }catch (e) {
    toast.add({
      title: 'Error',
      description: getErrorMessage(e),
      duration: 5000,
      color: 'error',
    })
    console.error('Error creating domain:', e)
  }
}

const columns: TableColumn<SuggestBlock>[] = [
  {
    accessorKey: 'id',
    header: 'id',
  },
  {
    accessorKey: 'domain',
    header: 'Domain'
  },
  {
    accessorKey: 'score',
    header: 'Score'
  },
  {
    accessorKey: 'reasons',
    header: () => h('div', 'Reason'),
    cell: (props) => {
      return h('ul', props.row.original.reasons.split('\n').map((reason: string) => {
        return h('li', reason)
      }))
    }
  }, {
    accessorKey: '1',
    header: () => h('div', 'Actions'),
    cell: (props) => {
      return h('div', [
        h(UButton, {
          size: 'sm',
          color: 'primary',
          onClick: async () => {
            await createDomain(props.row.original)
          }
        }, () => 'Apply Domain'),
      ])
    }
  }
]

</script>

<template>
  <UContainer>
      <div class="w-full space-y-4 pb-4">
        <div class="flex px-4 py-3.5 justify-between border-b border-accented">
          <UInput
              @change="changeFilter"
              v-model="globalFilter"
              class="max-w-sm"
              placeholder="Search" />
        </div>

        <UTable
          :loading="isLoading"
          empty="No data"
          v-model:pagination="pagination"
          :data="records"
          :columns="columns"
          class="flex-1"
      />

      <div class="flex justify-center border-t border-default pt-4">
        <UPagination
            :default-page="(pagination.pageIndex) + 1"
            :items-per-page="pagination.pageSize"
            :total="pagination.total"
            @update:page="changePage"
        />
      </div>
    </div>
  </UContainer>

</template>
