import type { ReactNode } from "react";

import { Shell } from "@/components/shell";

// Every page under this group renders inside the authenticated shell, which
// redirects anonymous visitors to /login.
export default function AppLayout({ children }: { children: ReactNode }) {
  return <Shell>{children}</Shell>;
}
