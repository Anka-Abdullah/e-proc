"use client";

// Client-side providers: React Query for server state and AuthProvider for the
// session. Kept in one component so the root layout stays a server component.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";

import { ApiError } from "@/lib/api";
import { AuthProvider } from "@/lib/auth";

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // A 401 will not fix itself by retrying; everything else gets one retry.
            retry: (failureCount, error) =>
              !(error instanceof ApiError && error.status < 500) && failureCount < 1,
            staleTime: 15_000,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={client}>
      <AuthProvider>{children}</AuthProvider>
    </QueryClientProvider>
  );
}
