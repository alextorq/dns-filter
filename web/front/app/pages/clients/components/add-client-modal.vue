<script setup lang="ts">
import {api} from '~/api'
import {useToggle} from "~~/composables/use-toggle";
import {useComponentStatus} from "~~/composables/use-component-status";
import {ComponentStatus} from "~~/utils/component-status";
import {getErrorMessage} from "~~/utils/get-error-message";

const emit = defineEmits(['success'])

const toast = useToast()
const {isActive, closeHandler, openHandler} = useToggle()
const {status} = useComponentStatus()


const validationError = ref('')

const state = reactive({
  user_id: ''
})

const validation = (): string | null => {
  if (state.user_id.length <= 0) {
    return 'User ID must be '
  }
  return null
}

const validateUserId = () => {
  validationError.value = validation() || ''
  return !validationError.value
}

const onSubmit = async (e: Event) => {
  if (validateUserId()) {
    try {
      status.value = ComponentStatus.LOADING
      e.preventDefault()
      await api.addExcludeClient({ user_id: state.user_id })
      toast.add({
        title: 'Success',
        description: 'New client was added.',
        duration: 3000,
      })
      emit('success')
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
        label="Add Client"
        />

    <template #header>
      <h1>Add Client</h1>
    </template>

    <template #body>
      <div>
        <UForm
            style="width: 600px;"
            @submit="onSubmit"
            :state="state"
            >

          <UFormField
              label="User ID"
              name="user_id"
              required>
            <UInput
                size="xl"
                @change="validateUserId"
                v-model="state.user_id"
                placeholder="192.168.88.88"
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
        <UButton @click="onSubmit" size="xl" label="Add Client" type="submit" />
      </div>
    </template>

  </UDrawer>
</template>

<style scoped>

</style>