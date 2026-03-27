import { Suspense } from 'react';

import LoginForm from '@/views/user/login';

export default function Page() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
