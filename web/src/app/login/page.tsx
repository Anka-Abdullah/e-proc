"use client";

// Login screen. Validation runs client-side with zod, but the server remains the
// authority: a rejected login is shown with the message the API returned.

import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Button, ErrorBanner, Field, Input } from "@/components/ui";

const schema = z.object({
  tenantId: z.string().min(1, "Tenant wajib diisi"),
  email: z.string().min(1, "Email wajib diisi").email("Format email tidak valid"),
  password: z.string().min(1, "Password wajib diisi"),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  const { login } = useAuth();
  const router = useRouter();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { tenantId: "demo", email: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setServerError(null);
    try {
      await login(values);
      router.replace("/dashboard");
    } catch (err) {
      if (err instanceof ApiError) {
        // Login failures are deliberately vague server-side; show what we got.
        setServerError(err.message);
        return;
      }
      setServerError(err instanceof Error ? err.message : "Login gagal");
    }
  });

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <header className="mb-5">
          <h1 className="text-lg font-semibold text-slate-900">E-Procurement</h1>
          <p className="mt-1 text-xs text-slate-500">Masuk untuk mengelola pengadaan</p>
        </header>

        {serverError && (
          <div className="mb-4">
            <ErrorBanner message={serverError} />
          </div>
        )}

        <form onSubmit={onSubmit} className="space-y-3">
          <Field label="Tenant" error={errors.tenantId?.message}>
            <Input placeholder="demo" autoComplete="organization" {...register("tenantId")} />
          </Field>
          <Field label="Email" error={errors.email?.message}>
            <Input placeholder="nama@perusahaan.co.id" autoComplete="username" {...register("email")} />
          </Field>
          <Field label="Password" error={errors.password?.message}>
            <Input type="password" autoComplete="current-password" {...register("password")} />
          </Field>

          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting ? "Memproses…" : "Masuk"}
          </Button>
        </form>

        <p className="mt-4 text-[11px] leading-relaxed text-slate-500">
          Belum punya akun? Minta administrator membuatkannya, atau gunakan{" "}
          <code className="rounded bg-slate-100 px-1">go run ./scripts/mint-token</code> untuk
          token uji di luar UI.
        </p>
      </div>
    </main>
  );
}
