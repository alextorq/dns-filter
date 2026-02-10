<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import {api, type SyncRecord} from "~/api";
import ChangeSyncStatus from '~/sync/change-sync-status/components/change-sync-status.vue';
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";

const toast = useToast()

let lastFetchController: AbortController | null = null

const data = ref<SyncRecord[]>([])

const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

useHead({
  title: 'Sources',
})

const pagination = ref({
  pageIndex: 0,
  pageSize: 12,
  total: 0,
})

const fetchData = async () => {
  try {
    if (lastFetchController) lastFetchController.abort()
    lastFetchController = new AbortController()
    const response = await api.getAllSyncRecords({
      limit: pagination.value.pageSize,
      offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
    }, lastFetchController.signal)

    data.value = response.list
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

const fetchWithLoading = createLoadingRequest(fetchData)

const changePage = async (page: number) => {
  pagination.value.pageIndex = page - 1
  await fetchWithLoading()
}

onMounted(fetchWithLoading)

const updateActiveStatus = (item: SyncRecord) => {
  try {
    const index = data.value.findIndex(record => record.id === item.id)
    if (index !== -1) {
      data.value.splice(index, 1, item)
    }
  } catch (error) {
    console.error('Error updating status:', error)
  }
}

const columns: TableColumn<SyncRecord>[] = [
  {
    accessorKey: 'id',
    header: 'id',
  },
  {
    accessorKey: 'created_at',
    header: 'Date of creation',
    cell: ({ row }) => {
      return new Date(row.getValue('created_at')).toLocaleString('en-En', {
        day: 'numeric',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      })
    }
  },
  {
    accessorKey: 'name',
    header: 'Name'
  },
  {
    accessorKey: 'url',
    header: 'URL',
  },
  {
    accessorKey: 'active',
    header: () => h('div', { class: 'text-right' }, 'Active'),
    cell: ({ row }) => {
      return h(ChangeSyncStatus, {
        record: row.original,
        onUpdate: updateActiveStatus,
      })
    }
  }
]

</script>

<template>
  <UContainer>
    <div class="w-full space-y-4 pb-4">
      <UTable
          :loading="isLoading"
          empty="No data"
          v-model:pagination="pagination"
          :data="data"
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
