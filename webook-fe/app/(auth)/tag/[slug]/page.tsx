import { Suspense } from 'react';

import TagDetailPage from '@/views/tag/detail';

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return (
    <Suspense>
      <TagDetailPage slug={decodeURIComponent(slug)} />
    </Suspense>
  );
}
