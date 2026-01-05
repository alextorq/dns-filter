<script setup lang="ts">
import type { TableColumn,  } from '@nuxt/ui'
import {api, type SuggestBlock} from "~/api";
import AddDomainModal from '~/domain/add-new-domain/components/add-domain-modal.vue';
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {UButton} from "#components";

let lastFetchController: AbortController | null = null

const data = ref<SuggestBlock[]>([])
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
const toast = useToast()

onMounted(fetchWithLoading)

const createDomain = async (item: SuggestBlock) => {
  try {
    await api.createDomain(item.domain)
    await api.changeSuggestStatus(item.id, false)
    await fetchWithLoading()
    toast.add({
      title: 'Success',
      description: 'New domain was added.',
      duration: 3000,
    })
  }catch (e) {
    console.log(e)
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
    accessorKey: 'reasons',
    header: () => h('div', 'Reason'),
  }, {
    accessorKey: '1',
    header: () => h('div', 'Actions'),
    cell: (props) => {
      return h('div', [
        // UButton
        // h(UButton, {
        //   size: 'sm',
        //   color: 'secondary',
        //   onClick: () => {
        //     updateActiveStatus(props.row.original)
        //   }
        // }, () =>  'Change Status'),

        h(UButton, {
          size: 'sm',
          color: 'primary',
          onClick: () => {
            createDomain(props.row.original)
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
