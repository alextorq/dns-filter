<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import {api, type DNSRecord} from "~/api";
import {USwitch} from "#components";
import AddDomainModal from '~/add-new-domain/components/add-domain-modal.vue';

const data = shallowRef<DNSRecord[]>([])
const globalFilter = ref()

const pagination = ref({
  pageIndex: 0,
  pageSize: 12,
  total: 0,
})

const fetchData = async () => {
  try {
    const response = await api.getAllDnsRecords({
      limit: pagination.value.pageSize,
      offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
      filter: globalFilter.value,
    })
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

const changeFilter = async () => {
  await fetchData()
}

const changePage = async (page: number) => {
  pagination.value.pageIndex = page - 1
  await fetchData()
}

onMounted(fetchData)


const updateActiveStatus = async (id: number, newStatus: boolean) => {
  try {
    const res = await api.changeDnsRecordStatus(id, newStatus)
    const index = data.value.findIndex(record => record.id === id)
    if (index !== -1) {
      data.value.splice(index, 1, res)
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
    header: 'Дата создания',
    cell: ({ row }) => {
      return new Date(row.getValue('created_at')).toLocaleString('ru-RU', {
        day: 'numeric',
        month: 'long',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
      })
    }
  },
  {
    accessorKey: 'url',
    header: 'url'
  },
  {
    accessorKey: 'active',
    header: () => h('div', { class: 'text-right' }, 'Активен'),
    cell: ({ row }) => {
      const amount = row.getValue('active')

      return h(USwitch, {
        'unchecked-icon': "i-lucide-x",
        'checked-icon': "i-lucide-check",
        'default-value': amount,
        'onUpdate:modelValue': (newValue: boolean) => {
          updateActiveStatus(row.original.id, newValue)
        },
        class: 'justify-end'
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
          ref="table"
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
