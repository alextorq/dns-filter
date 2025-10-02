<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import {api, type DNSRecord} from "~/api";
import AddDomainModal from '~/domain/add-new-domain/components/add-domain-modal.vue';
import ChangeStatus from '~/domain/change-domain-status/components/change-status.vue';
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";

let lastFetchController: AbortController | null = null

const data = ref<DNSRecord[]>([])
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
    const response = await api.getAllDnsRecords({
      limit: pagination.value.pageSize,
      offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
      filter: globalFilter.value,
    }, lastFetchController.signal)

    data.value = response.list
    pagination.value = {
      ...pagination.value,
      pageIndex: 0,
      total: response.total,
    }
  } catch (error) {
    console.error('Error fetching data:', error)
  }
}

const fetchWithLoading = createLoadingRequest(fetchData)

const changeFilter = async () => {
  await fetchWithLoading()
}

const changePage = async (page: number) => {
  pagination.value.pageIndex = page - 1
  await fetchWithLoading()
}

onMounted(fetchWithLoading)

const updateActiveStatus = (item: DNSRecord) => {
  try {
    const index = data.value.findIndex(record => record.id === item.id)
    if (index !== -1) {
      data.value.splice(index, 1, item)
    }
  } catch (error) {
    console.error('Error updating status:', error)
  }
}

const columns: TableColumn<DNSRecord>[] = [
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
    accessorKey: 'url',
    header: 'Domain'
  },
  {
    accessorKey: 'active',
    header: () => h('div', { class: 'text-right' }, 'Active'),
    cell: ({ row }) => {
      return h(ChangeStatus, {
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
      <div class="flex px-4 py-3.5 justify-between border-b border-accented">
        <UInput
            @change="changeFilter"
            v-model="globalFilter"
            class="max-w-sm"
            placeholder="Search" />
        <AddDomainModal/>
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
