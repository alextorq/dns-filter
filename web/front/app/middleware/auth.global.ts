import { useAuth } from "~~/composables/use-auth";

export default defineNuxtRouteMiddleware(async (to) => {
    const { user, ready, fetchMe } = useAuth();
    if (!ready.value) {
        await fetchMe();
    }

    const isAuthRoute = to.path.startsWith("/auth");

    if (!user.value && !isAuthRoute) {
        return navigateTo("/auth");
    }
    if (user.value && isAuthRoute) {
        return navigateTo("/");
    }
});
