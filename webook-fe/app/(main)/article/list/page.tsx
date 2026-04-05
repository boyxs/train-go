import { Suspense } from 'react';

import { Loading } from '@/components/common/Loading';
import ArticleListPage from '@/views/article/list';

export default function Page() {
  return (
    <Suspense fallback={<Loading />}>
      <ArticleListPage />
    </Suspense>
  );
}
