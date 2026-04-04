import { Suspense } from 'react';

import SearchPage from '@/views/search';

export default function Page() {
  return (
    <Suspense>
      <SearchPage />
    </Suspense>
  );
}
