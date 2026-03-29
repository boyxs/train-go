import { Suspense } from 'react';

import { Loading } from '@/components/common/Loading';
import ArticleFeedPage from '@/views/article/feed';

export default function Page() {
  return (
    <Suspense fallback={<Loading />}>
      <ArticleFeedPage />
    </Suspense>
  );
}
