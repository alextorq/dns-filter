import {ref} from "vue";

export const useToggle = (initialValue: boolean = false) => {
    const isActive = ref(initialValue);

    const toggle = () => {
        isActive.value = !isActive.value;
    };

    const openHandler = () => {
        isActive.value = true;
    };

    const closeHandler = () => {
        isActive.value = false;
    };

    return {
        openHandler,
        toggle,
        closeHandler,
        isActive
    };
}