<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import {api, type ExcludeClient} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";
import AddClientModal from "./components/add-client-modal.vue";
import ChangeClientStatus from "./components/change-client-status.vue";
import DeleteClient from "./components/delete-client.vue";

const toast = useToast()

let lastFetchController: AbortController | null = null

const data = ref<ExcludeClient[]>([])

const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

useHead({
  title: 'Exclude Clients',
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
    const response = await api.getAllExcludeClients({
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

const updateActiveStatus = (item: ExcludeClient) => {
  try {
    const index = data.value.findIndex(record => record.id === item.id)
    if (index !== -1) {
      data.value.splice(index, 1, item)
    }
  } catch (error) {
    console.error('Error updating status:', error)
  }
}

const deleteClient = (item: ExcludeClient) => {
  try {
    const index = data.value.findIndex(record => record.id === item.id)
    if (index !== -1) {
      data.value.splice(index, 1)
    }
  } catch (error) {
    console.error('Error deleting client:', error)
  }
}

const columns: TableColumn<ExcludeClient>[] = [
  {
    accessorKey: 'id',
    header: 'ID',
  },
  {
    accessorKey: 'user_id',
    header: 'User ID',
  },
  {
    accessorKey: 'created_at',
    header: 'Created At',
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
    accessorKey: 'updated_at',
    header: 'Updated At',
    cell: ({ row }) => {
      return new Date(row.getValue('updated_at')).toLocaleString('en-En', {
        day: 'numeric',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      })
    }
  },
  {
    accessorKey: 'active',
    header: 'Active',
    cell: ({ row }) => {
      return h(ChangeClientStatus, {
        record: row.original,
        onUpdate: updateActiveStatus,
      })
    }
  },
  {
    id: 'actions',
    header: '',
    cell: ({ row }) => {
      return h(DeleteClient, {
        record: row.original,
        onDelete: deleteClient,
      })
    }
  }
]
</script>

<template>
  <UContainer>
    <div class="w-full space-y-4 pb-4">
      <div class="flex px-4 py-3.5 justify-end border-b border-accented">
        <AddClientModal @success="fetchWithLoading" />
      </div>

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
