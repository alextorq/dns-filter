import {useComponentStatus} from "./use-component-status";
import {ComponentStatus} from "../utils/component-status";

export const useComponentStatusWithLoading = () => {
    const {status, isLoading, isErrorLoading, isResult, isSaving, isErrorSaving, isInitial} = useComponentStatus()

    const createLoadingRequest = (loadingFunction: () => Promise<void>) => {
       const fn = async () => {
           status.value = ComponentStatus.LOADING;
           try {
               await loadingFunction();
               status.value = ComponentStatus.RESULT;
           } catch (e) {
               status.value = ComponentStatus.ERROR_LOADING;
               throw e;
           }
       }
       return fn
    }

    return {
        status,
        isLoading,
        isErrorLoading,
        isResult,
        isSaving,
        isErrorSaving,
        isInitial,
        createLoadingRequest
    }

}