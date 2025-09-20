import {ComponentStatus} from "../utils/component-status";
import {ref, computed} from 'vue'

export const useComponentStatus = () => {
    const status = ref<ComponentStatus>(ComponentStatus.INITIAL)

    const isLoading = computed(() => status.value === ComponentStatus.LOADING)
    const isErrorLoading = computed(() => status.value === ComponentStatus.ERROR_LOADING)
    const isResult = computed(() => status.value === ComponentStatus.RESULT)
    const isSaving = computed(() => status.value === ComponentStatus.SAVING)
    const isErrorSaving = computed(() => status.value === ComponentStatus.ERROR_SAVING)
    const isInitial = computed(() => status.value === ComponentStatus.INITIAL)


    return {
        status,
        isLoading,
        isErrorLoading,
        isResult,
        isSaving,
        isErrorSaving,
        isInitial
    }
}