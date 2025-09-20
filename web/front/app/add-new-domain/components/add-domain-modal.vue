<script setup lang="ts">
import {api} from '~/api'
import {isAxiosError} from "axios";
import {useToggle} from "~~/composables/use-toggle";
import {useComponentStatus} from "~~/composables/use-component-status";
import {ComponentStatus} from "~~/utils/component-status";

const toast = useToast()
const {isActive, closeHandler, openHandler} = useToggle()
const {status} = useComponentStatus()


const state = reactive({
  domain: ''
})

const onSubmit = async (e: Event) => {
  try {
    status.value = ComponentStatus.LOADING
    e.preventDefault()
    await api.createDomain(state.domain)
    toast.add({
      title: 'Success',
      description: 'New domain was added.',
      duration: 3000,
    })
    closeHandler()
  }catch (e) {
    status.value = ComponentStatus.ERROR_LOADING
    if (isAxiosError(e)) {
      const response = e.response
      if (response && response.data && response.data.message) {
        toast.add({
          title: 'Error',
          description: response.data.message,
          duration: 5000,
        })
        return
      }
      console.log(e)
    }
  }
}
</script>

<template>
  <UDrawer
      v-model:open="isActive"
      direction="right">
    <UButton
        @click="openHandler"
        label="Add domain"
        />

    <template #header>
      <h1>Add Domain</h1>
    </template>

    <template #body>
      <div>
        <UForm
            style="width: 600px;"
            @submit="onSubmit"
            :state="state"
            >

          <UFormField
              label="Domain"
              name="domain"
              required>
            <UInput
                size="xl"
                v-model="state.domain"
                placeholder="google.com."
                required />

          </UFormField>
        </UForm>

      </div>
    </template>

    <template #footer>
      <div class="flex justify-start">
        <UButton @click="onSubmit" size="xl" label="Add domain" type="submit" />
      </div>
    </template>

  </UDrawer>
</template>

<style scoped>

</style>