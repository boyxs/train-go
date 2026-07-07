import { Suspense } from 'react';

import { Loading } from '@/components/common/Loading';
import ProfilePage from '@/views/user/profile';

export default function Page() {
  return (
    <Suspense fallback={<Loading />}>
      <ProfilePage />
    </Suspense>
  );
}
