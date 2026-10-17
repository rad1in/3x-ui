import type { Meta, StoryObj } from '@storybook/react-vite';
import { MemoryRouter } from 'react-router';
import { Button } from 'antd';
import { ImportOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons';

import EmptyState from './EmptyState';
import GettingStarted from './GettingStarted';

const meta = {
  title: 'Feedback/EmptyState',
  component: EmptyState,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          'Placeholder for an empty list. Explains what belongs here and offers the next action; a list emptied by search or filters gets a short "no matches" variant with a clear button instead. Children render below the action, e.g. the GettingStarted checklist.',
      },
    },
  },
  argTypes: {
    icon: { description: 'Glyph shown in the framed badge above the title.' },
    title: { description: 'One-line headline.' },
    description: { description: 'Optional explanation under the title.' },
    action: { description: 'Optional primary call to action.' },
    children: { description: 'Optional extra content below the action.' },
  },
  args: {
    icon: <ImportOutlined />,
    title: 'No inbounds yet',
    description:
      'An inbound is the port and protocol your clients connect to. Create one to start serving traffic.',
    action: (
      <Button type="primary" icon={<PlusOutlined />}>
        Add Inbound
      </Button>
    ),
  },
} satisfies Meta<typeof EmptyState>;

export default meta;

type Story = StoryObj<typeof meta>;

export const FirstRun: Story = {
  args: { children: <GettingStarted hasInbound={false} hasClient={false} /> },
};

export const SecondStep: Story = {
  args: {
    title: 'No clients yet',
    children: <GettingStarted hasInbound hasClient={false} />,
  },
};

export const NoMatches: Story = {
  args: {
    icon: <SearchOutlined />,
    title: 'No results match your search or filters',
    description: undefined,
    action: <Button>Clear</Button>,
  },
};
