<script setup lang="ts">
import {api} from '~/api'
import {useToggle} from "~~/composables/use-toggle";
import {useComponentStatus} from "~~/composables/use-component-status";
import {ComponentStatus} from "~~/utils/component-status";
import {getErrorMessage} from "~~/utils/get-error-message";

const toast = useToast()
const {isActive, closeHandler, openHandler} = useToggle()
const {status} = useComponentStatus()


const validationError = ref('')

const state = reactive({
  domain: ''
})

const validation = (): string | null => {
  const domain = state.domain.trim()
  if (!domain) {
    return 'Domain is required.'
  }

  if (domain.length > 253) {
    return 'Domain must not exceed 253 characters.'
  }

  if (!domain.endsWith('.')) {
    return 'Domain must end with a dot (.)'
  }

  // убираем последнюю точку для проверки частей
  const labels = domain.slice(0, -1).split('.')

  for (const label of labels) {
    if (label.length < 1) {
      return 'Each domain label must be at least 1 character.'
    }
    if (label.length > 63) {
      return 'Each domain label must not exceed 63 characters.'
    }
    if (!/^[a-zA-Z0-9-]+$/.test(label)) {
      return 'Labels may only contain letters, digits, and hyphens.'
    }
    if (label.startsWith('-') || label.endsWith('-')) {
      return 'Labels must not start or end with a hyphen.'
    }
  }

  return null
}

const validateDomain = () => {
  validationError.value = validation() || ''
  return !validationError.value
}

const onSubmit = async (e: Event) => {
  if (validateDomain()) {
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
      console.error('Error', e)
      const message = getErrorMessage(e)
      toast.add({
        title: 'Error',
        description: message,
        duration: 5000,
        color: 'error',
      })
    }
  }

}
</script>

<template>
  <UDrawer
      v-model:open="isActive"
      direction="right">
    <UButton
        size="xl"
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
                @change="validateDomain"
                v-model="state.domain"
                placeholder="google.com."
                required />

            <div v-if="validationError">
              {{validationError}}
            </div>

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