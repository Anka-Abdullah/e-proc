import { redirect } from "next/navigation";

// The root has no screen of its own: send visitors to the dashboard, which sends
// anonymous visitors on to the login page.
export default function Home() {
  redirect("/dashboard");
}
