<script setup lang="ts">
import {api} from '~/api'
import {isAxiosError} from "axios";
import {useToggle} from "~~/composables/use-toggle";
const toast = useToast()
const {isActive, closeHandler, openHandler} = useToggle()

const state = reactive({
  domain: ''
})

const onSubmit = async (e: Event) => {
  try {
    e.preventDefault()
    await api.createDomain(state.domain)
    toast.add({
      title: 'Form Submitted',
      description: 'Check the console for details.',
      duration: 3000,
    })
    closeHandler()
  }catch (e) {
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
            class="space-y-4">

          <UFormField
              label="Domain"
              name="domain"
              required>
            <UInput
                v-model="state.domain"
                placeholder="google.com."
                required />

          </UFormField>
        </UForm>

      </div>
    </template>

    <template #footer>
      <div class="flex justify-start">
        <UButton @click="onSubmit" label="Add" type="submit" />
      </div>
    </template>

  </UDrawer>
</template>

<style scoped>

</style>