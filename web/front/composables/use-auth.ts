import { api, type CurrentUser } from "~/api";

let inflight: Promise<CurrentUser | null> | null = null;

export const useAuth = () => {
    const user = useState<CurrentUser | null>("auth.user", () => null);
    const ready = useState<boolean>("auth.ready", () => false);

    const fetchMe = (): Promise<CurrentUser | null> => {
        if (ready.value) {
            return Promise.resolve(user.value);
        }
        if (inflight) {
            return inflight;
        }
        inflight = api
            .me()
            .then((u) => {
                user.value = u;
                return u;
            })
            .catch(() => {
                user.value = null;
                return null;
            })
            .finally(() => {
                ready.value = true;
                inflight = null;
            });
        return inflight;
    };

    const login = async (loginValue: string, password: string) => {
        const u = await api.login(loginValue, password);
        user.value = u;
        ready.value = true;
        return u;
    };

    const logout = async () => {
        try {
            await api.logout();
        } finally {
            user.value = null;
        }
    };

    const setUnauthenticated = () => {
        user.value = null;
        ready.value = true;
    };

    return { user, ready, fetchMe, login, logout, setUnauthenticated };
};
