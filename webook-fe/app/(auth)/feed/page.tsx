import { Suspense } from 'react';

import { Loading } from '@/components/common/Loading';
import FeedTabsPage from '@/views/feed';

export default function Page() {
  return (
    <Suspense fallback={<Loading />}>
      <FeedTabsPage />
    </Suspense>
  );
}
